import Foundation

/// 외부 명령 결과.
public struct CommandResult: Sendable, Equatable {
  public let stdout: String
  public let stderr: String
  public let exitCode: Int32
}

public enum CommandError: Error, Equatable, CustomStringConvertible {
  /// 실행 파일이 없음 — 스캔에서는 에러가 아니라 "그 도구는 안 깔려 있음"(조용히 건너뜀)
  case notFound(String)
  case failed(command: String, exitCode: Int32, detail: String)
  case timedOut(command: String, seconds: Double)

  public var description: String {
    switch self {
    case .notFound(let n): return "\(n): 없음"
    case .failed(let c, let code, let d): return "\(c) 실패(종료 코드 \(code))" + (d.isEmpty ? "" : ": \(d)")
    case .timedOut(let c, let s): return "\(c): \(Int(s))초 안에 끝나지 않아 중단"
    }
  }
}

/// 외부 명령 실행기.
/// - 모든 호출에 타임아웃.
/// - 메뉴 막대 앱은 셸 환경을 물려받지 않으므로 `searchPath`로 실행 파일을 찾고 자식 프로세스의 PATH도 그걸로 준다.
/// - **읽기**(기본): 종료 코드가 허용 밖이어도 stdout이 있으면 성공(`npm ls`/`npm outdated`는 정상에도 exit 1).
/// - **변경**(`strict: true`): 종료 코드로만 판정 — 부분 실패를 성공으로 오인하지 않는다. 실패 메시지엔 stderr(없으면 stdout).
public struct CommandRunner: Sendable {
  public var searchPath: [String]
  public var extraEnvironment: [String: String]

  public init(searchPath: [String], extraEnvironment: [String: String] = [:]) {
    self.searchPath = searchPath
    self.extraEnvironment = extraEnvironment
  }

  /// 이름(또는 절대 경로)을 실행 파일 경로로. 없으면 nil.
  public func resolve(_ name: String) -> String? {
    if name.contains("/") { return FileManager.default.isExecutableFile(atPath: name) ? name : nil }
    return SearchPath.find(name, in: searchPath) { FileManager.default.isExecutableFile(atPath: $0) }
  }

  public func run(_ name: String, _ args: [String] = [], timeout: Double = 120,
                  okCodes: Set<Int32> = [0], strict: Bool = false) async throws -> CommandResult {
    guard let exe = resolve(name) else { throw CommandError.notFound(name) }
    let label = ([name] + args).joined(separator: " ")

    var env = ProcessInfo.processInfo.environment
    env["PATH"] = searchPath.joined(separator: ":")
    env.merge(extraEnvironment) { $1 }
    let out = Pipe(), err = Pipe()
    // 새 프로세스 그룹으로 시작한다 — 타임아웃 때 자식이 띄운 하위 프로세스(brew → ruby·curl …)까지 함께 끄려고.
    // Foundation Process는 그룹을 정할 수 없어 posix_spawn을 쓴다(2026-11-21 점검: 바로 아래 프로세스만 끄면
    // 하위 프로세스가 파이프를 잡고 있어 읽기가 끝나지 않았다).
    let pid = try Self.spawn(exe, args: args, env: env, stdout: out.fileHandleForWriting.fileDescriptor,
                             stderr: err.fileHandleForWriting.fileDescriptor)
    // 부모 쪽 쓰기 끝을 닫아야 자식들이 다 끝났을 때 EOF가 온다
    try? out.fileHandleForWriting.close()
    try? err.fileHandleForWriting.close()

    let timedOut = Flag()
    let reads = Reads(out: out.fileHandleForReading, err: err.fileHandleForReading)
    // 종료는 waitpid로(GCD 스레드). waitUntilExit()는 쓰지 않는다 — 시작한 스레드의 런루프로 종료를 받는데
    // await 뒤엔 다른 스레드에서 이어질 수 있어 영영 못 받고 멈췄다(2026-11-21 실측).
    let exit = Task.detached { await Self.waitExit(pid) }
    // 바로 아래 프로세스가 끝났는데 손자가 파이프를 잡고 있으면(정상 백그라운드 — brew services 등) 제한 시간까지 기다리지 않는다:
    // 몇 초만 더 읽고, 안 끝나면 그때까지 읽은 출력과 실제 종료 코드로 판정한다. 손자는 건드리지 않는다(2026-11-21 점검)
    let grace = Task.detached {
      _ = await exit.value
      try? await Task.sleep(for: .seconds(Self.drainGrace))
      if !Task.isCancelled { reads.abandon() }
    }
    let watchdog = Task.detached {
      try? await Task.sleep(for: .seconds(timeout))
      guard !Task.isCancelled else { return }
      timedOut.set()
      // 그룹 전체에 정중히 → 2초 뒤에도 남았으면 강제로. 바로 아래 프로세스가 이미 끝났어도 그룹은 남아 있을 수 있다.
      killpg(pid, SIGTERM)
      try? await Task.sleep(for: .seconds(2))
      killpg(pid, SIGKILL)
      // 그룹을 빠져나간 손자가 파이프를 계속 잡고 있어도 멈추지 않는다 — 읽기를 포기하고 돌아온다
      try? await Task.sleep(for: .seconds(3))
      reads.abandon()
    }
    let (od, ed) = await reads.wait()
    let status = await exit.value
    watchdog.cancel()
    grace.cancel()

    if timedOut.value { throw CommandError.timedOut(command: label, seconds: timeout) }
    let result = CommandResult(stdout: String(decoding: od, as: UTF8.self),
                               stderr: String(decoding: ed, as: UTF8.self),
                               exitCode: status)
    if okCodes.contains(result.exitCode) { return result }
    if !strict, !result.stdout.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return result }
    let detail = (result.stderr.isEmpty ? result.stdout : result.stderr).trimmingCharacters(in: .whitespacesAndNewlines)
    throw CommandError.failed(command: label, exitCode: result.exitCode, detail: String(detail.prefix(500)))
  }

  /// posix_spawn: 새 프로세스 그룹(자기 pid), stdin은 /dev/null(sudo 같은 대화형 입력은 바로 실패 — 멈추지 않음),
  /// stdout·stderr만 넘기고 나머지 fd는 닫는다. 시그널은 기본값으로(GUI 앱이 무시하는 SIGPIPE 등을 물려주지 않게).
  static func spawn(_ exe: String, args: [String], env: [String: String], stdout: Int32, stderr: Int32) throws -> pid_t {
    var attr: posix_spawnattr_t?
    posix_spawnattr_init(&attr)
    defer { posix_spawnattr_destroy(&attr) }
    posix_spawnattr_setflags(&attr, Int16(POSIX_SPAWN_SETPGROUP | POSIX_SPAWN_CLOEXEC_DEFAULT | POSIX_SPAWN_SETSIGDEF | POSIX_SPAWN_SETSIGMASK))
    posix_spawnattr_setpgroup(&attr, 0)
    var none = sigset_t(), all = sigset_t()
    sigemptyset(&none); sigfillset(&all)
    posix_spawnattr_setsigmask(&attr, &none)
    posix_spawnattr_setsigdefault(&attr, &all)

    var fa: posix_spawn_file_actions_t?
    posix_spawn_file_actions_init(&fa)
    defer { posix_spawn_file_actions_destroy(&fa) }
    posix_spawn_file_actions_addopen(&fa, 0, "/dev/null", O_RDONLY, 0)
    posix_spawn_file_actions_adddup2(&fa, stdout, 1)
    posix_spawn_file_actions_adddup2(&fa, stderr, 2)

    let argv = ([exe] + args).map { strdup($0) } + [nil]
    let envp = env.map { strdup("\($0.key)=\($0.value)") } + [nil]
    defer { argv.forEach { free($0) }; envp.forEach { free($0) } }
    var pid: pid_t = 0
    let rc = posix_spawn(&pid, exe, &fa, &attr, argv, envp)
    guard rc == 0 else { throw CommandError.failed(command: exe, exitCode: -1, detail: String(cString: strerror(rc))) }
    return pid
  }

  /// 바로 아래 프로세스가 끝난 뒤 남은 출력을 더 기다리는 시간(초)
  static let drainGrace: Double = 3

  /// 끝날 때까지 기다려 종료 코드를 돌려준다(시그널로 끝났으면 128+번호 — 셸과 같은 관례). GCD 스레드에서 막힌다.
  /// waitpid가 EINTR 말고 다른 오류를 내면 **-1** — 종료 상태를 모르는데 0(성공)으로 보면 안 된다(엄격 모드에서 실패가 되게).
  static func waitExit(_ pid: pid_t) async -> Int32 {
    await withCheckedContinuation { c in
      DispatchQueue.global(qos: .utility).async { c.resume(returning: reap(pid)) }
    }
  }

  static func reap(_ pid: pid_t) -> Int32 {
    var st: Int32 = 0
    var r: pid_t
    repeat { r = waitpid(pid, &st, 0) } while r == -1 && errno == EINTR
    guard r == pid else { return -1 }
    let sig = st & 0x7f
    return sig == 0 ? (st >> 8) & 0xff : 128 + sig
  }
}

/// stdout·stderr를 끝까지 읽는다(블로킹 읽기는 GCD 스레드에서 — Swift 동시성 스레드 풀을 막으면 동시 실행이 많을 때
/// 전체가 멈춘다). 조금씩 쌓아 두므로 `abandon()`을 부르면 **그때까지 읽은 것**으로 돌아온다(한 번만 응답).
private final class Reads: @unchecked Sendable {
  private let lock = NSLock()
  private var out = Data(), err = Data(), pending = 2, finished = false
  private var waiter: CheckedContinuation<(Data, Data), Never>?

  init(out o: FileHandle, err e: FileHandle) {
    for (h, isOut) in [(o, true), (e, false)] {
      DispatchQueue.global(qos: .utility).async { [self] in
        while true {
          let chunk = h.availableData   // 데이터가 올 때까지 막힘, EOF면 빈 값
          if chunk.isEmpty { break }
          lock.lock()
          if isOut { out.append(chunk) } else { err.append(chunk) }
          lock.unlock()
        }
        lock.lock()
        pending -= 1
        let done = pending == 0
        lock.unlock()
        if done { finish() }
      }
    }
  }

  func wait() async -> (Data, Data) {
    await withCheckedContinuation { c in
      lock.lock()
      if finished { lock.unlock(); c.resume(returning: (out, err)); return }
      waiter = c
      lock.unlock()
    }
  }

  func abandon() { finish() }

  private func finish() {
    lock.lock()
    guard !finished else { lock.unlock(); return }
    finished = true
    let c = waiter, r = (out, err)
    waiter = nil
    lock.unlock()
    c?.resume(returning: r)
  }
}

private final class Flag: @unchecked Sendable {
  private let lock = NSLock()
  private var v = false
  func set() { lock.lock(); v = true; lock.unlock() }
  var value: Bool { lock.lock(); defer { lock.unlock() }; return v }
}

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

    let p = Process()
    p.executableURL = URL(fileURLWithPath: exe)
    p.arguments = args
    var env = ProcessInfo.processInfo.environment
    env["PATH"] = searchPath.joined(separator: ":")
    env.merge(extraEnvironment) { $1 }
    p.environment = env
    p.standardInput = FileHandle.nullDevice
    let out = Pipe(), err = Pipe()
    p.standardOutput = out
    p.standardError = err

    // 종료는 terminationHandler로 받는다(run() 전에 걸어야 놓치지 않는다).
    // waitUntilExit()는 쓰지 않는다 — 프로세스를 시작한 스레드의 런루프로 종료를 받는데, await 뒤엔 다른 스레드에서
    // 이어질 수 있어 영영 못 받고 멈췄다(2026-11-21 앱 데이터 측정에서 실측).
    let (exits, exitCont) = AsyncStream.makeStream(of: Int32.self, bufferingPolicy: .bufferingNewest(1))
    p.terminationHandler = { proc in exitCont.yield(proc.terminationStatus); exitCont.finish() }
    try p.run()
    let timedOut = Flag()
    let watchdog = Task.detached {
      try? await Task.sleep(for: .seconds(timeout))
      if !Task.isCancelled, p.isRunning { timedOut.set(); p.terminate() }
    }
    // 파이프는 따로 끝까지 읽는다(버퍼가 차면 자식이 멈추므로 종료를 먼저 기다리면 안 됨)
    async let o = Self.readAll(out.fileHandleForReading)
    async let e = Self.readAll(err.fileHandleForReading)
    let (od, ed) = await (o, e)
    var status: Int32 = -1
    for await s in exits { status = s }
    watchdog.cancel()

    let result = CommandResult(stdout: String(decoding: od, as: UTF8.self),
                               stderr: String(decoding: ed, as: UTF8.self),
                               exitCode: status)
    if timedOut.value { throw CommandError.timedOut(command: label, seconds: timeout) }
    if okCodes.contains(result.exitCode) { return result }
    if !strict, !result.stdout.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return result }
    let detail = (result.stderr.isEmpty ? result.stdout : result.stderr).trimmingCharacters(in: .whitespacesAndNewlines)
    throw CommandError.failed(command: label, exitCode: result.exitCode, detail: String(detail.prefix(500)))
  }

  /// 블로킹 읽기는 GCD 스레드에서 — Swift 동시성 스레드 풀(코어 수만큼)을 막으면 동시 실행이 많을 때 전체가 멈춘다.
  private static func readAll(_ h: FileHandle) async -> Data {
    await withCheckedContinuation { c in
      DispatchQueue.global(qos: .utility).async { c.resume(returning: h.readDataToEndOfFile()) }
    }
  }
}

private final class Flag: @unchecked Sendable {
  private let lock = NSLock()
  private var v = false
  func set() { lock.lock(); v = true; lock.unlock() }
  var value: Bool { lock.lock(); defer { lock.unlock() }; return v }
}

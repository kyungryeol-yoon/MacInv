import Foundation

/// 실행 로그 한 줄.
public struct ExecLine: Sendable, Identifiable, Hashable {
  public enum State: Sendable, Hashable { case running, ok, skipped, failed }
  public let id: Int
  public let target: String
  public let text: String
  public var state: State
  public var detail: String?
}

public struct ExecSummary: Sendable, Equatable {
  public var ok = 0
  public var failed = 0
  public var skipped = 0
  /// 휴지통으로 옮긴 것이 있음 — "비우기 전까지 실제로 안 빈다"
  public var trashed = 0
  /// MacInv 자신을 업데이트함 — 다시 시작해야 새 버전
  public var selfUpdated = false
}

/// 계획 실행 — **명시적 승인 뒤에만** 부른다. 한 번에 하나씩, 줄 단위 로그.
/// - 삭제는 휴지통으로만(rm 없음). 실행 직전에 보호 경로·MacInv 자신·실행 중 여부를 **한 번 더** 확인한다
/// - 명령은 엄격 판정(종료 코드). 실패 메시지에 stderr. 한 대상이 실패해도 다음 대상은 계속(실패 건수로 요약)
/// - 이미 없는 경로는 실패가 아니라 건너뜀(부모를 먼저 옮겼거나 사용자가 지웠음)
public struct Executor: Sendable {
  public var run: @Sendable (_ argv: [String], _ timeout: TimeInterval) async throws -> CommandResult
  public var trash: @Sendable (String) throws -> Void
  public var exists: @Sendable (String) -> Bool
  /// 이 경로를 지금 쓰고 있는 실행 중 앱 이름(있으면 건너뜀)
  public var runningOwner: @Sendable (String) async -> String?
  public var me: SelfIdentity

  public init(run: @escaping @Sendable ([String], TimeInterval) async throws -> CommandResult,
              trash: @escaping @Sendable (String) throws -> Void = Executor.moveToTrash,
              exists: @escaping @Sendable (String) -> Bool = { FileManager.default.fileExists(atPath: $0) || (try? FileManager.default.destinationOfSymbolicLink(atPath: $0)) != nil },
              runningOwner: @escaping @Sendable (String) async -> String? = { _ in nil },
              me: SelfIdentity) {
    self.run = run; self.trash = trash; self.exists = exists; self.runningOwner = runningOwner; self.me = me
  }

  /// 실제 실행기로 명령을 돌린다(엄격 판정). brew가 대화형으로 묻지 않게 한다.
  public static func commands(_ runner: CommandRunner) -> @Sendable ([String], TimeInterval) async throws -> CommandResult {
    var r = runner
    r.extraEnvironment.merge(["HOMEBREW_NO_ENV_HINTS": "1", "NONINTERACTIVE": "1", "HOMEBREW_NO_INSTALL_CLEANUP": "1"]) { $1 }
    let fixed = r
    return { argv, timeout in try await fixed.run(argv[0], Array(argv.dropFirst()), timeout: timeout, strict: true) }
  }

  @Sendable public static func moveToTrash(_ path: String) throws {
    try FileManager.default.trashItem(at: URL(fileURLWithPath: path), resultingItemURL: nil)
  }

  /// 실행. `low`는 사용자가 고른 [확신낮음] 경로(나머지 낮음은 뺀다). `log`는 줄이 바뀔 때마다 불린다.
  public func execute(_ plan: Plan, low: Set<String>, log: @escaping @Sendable (ExecLine) async -> Void) async -> ExecSummary {
    let plan = plan.selecting(low: low)
    var sum = ExecSummary(), n = 0
    func emit(_ target: String, _ text: String, _ state: ExecLine.State, _ detail: String? = nil) async -> Int {
      n += 1
      await log(ExecLine(id: n, target: target, text: text, state: state, detail: detail))
      return n
    }
    func finish(_ id: Int, _ target: String, _ text: String, _ state: ExecLine.State, _ detail: String? = nil) async {
      switch state { case .ok: sum.ok += 1; case .failed: sum.failed += 1; case .skipped: sum.skipped += 1; case .running: break }
      await log(ExecLine(id: id, target: target, text: text, state: state, detail: detail))
    }

    for t in plan.targets {
      // 앱 삭제·cask 삭제: 실행 중이면 대상 전체를 건너뛴다(번들을 지우면 켜진 앱이 깨진다).
      // cask는 `uninstall quit:`이 없으면 brew가 앱을 끄지 않고 번들을 지운다(2026-11-21 점검) — 앱 삭제와 같은 확인
      if isRemoval(t), let app = await runningApp(t) {
        let id = await emit(t.name, "\(t.name) 삭제", .running)
        await finish(id, t.name, "\(t.name) 삭제", .skipped, "\(app)이(가) 실행 중 — 앱을 끈 뒤 다시 하세요")
        continue
      }
      var commandFailed = false
      for argv in t.commands {
        let text = shellLine(argv)
        let id = await emit(t.name, text, .running)
        do {
          _ = try await run(argv, t.timeout)
          await finish(id, t.name, text, .ok)
          if t.kind == .brewCask, me.isSelf(caskToken: t.name), argv.contains("upgrade") { sum.selfUpdated = true }
        } catch {
          commandFailed = true
          await finish(id, t.name, text, .failed, Self.message(error))
        }
      }
      // 명령(예: brew uninstall)이 실패했으면 그 대상의 잔여물은 옮기지 않는다 — 앱은 남았는데 설정만 사라지면 안 된다
      if commandFailed && !t.trash.isEmpty {
        let id = await emit(t.name, "남은 파일 \(t.trash.count)개", .running)
        await finish(id, t.name, "남은 파일 \(t.trash.count)개", .skipped, "앞 명령이 실패해 옮기지 않았습니다")
        continue
      }
      for p in t.trash {
        let text = "휴지통으로: " + p.path
        let id = await emit(t.name, text, .running)
        if let why = refuse(p.path) { await finish(id, t.name, text, .failed, why); continue }
        if !exists(p.path) { await finish(id, t.name, text, .skipped, "이미 없음"); continue }
        if t.kind != .app, let app = await runningOwner(p.path) { await finish(id, t.name, text, .skipped, "\(app) 실행 중 — 앱을 끈 뒤 다시"); continue }
        do {
          try trash(p.path)
          sum.trashed += 1
          await finish(id, t.name, text, .ok)
        } catch {
          await finish(id, t.name, text, .failed, Self.message(error))
        }
      }
    }
    return sum
  }

  /// 앱을 없애는 대상인가 — 앱 삭제, 또는 `brew uninstall --cask`(업그레이드는 아님)
  func isRemoval(_ t: PlanTarget) -> Bool {
    t.kind == .app || (t.kind == .brewCask && t.commands.contains { $0.starts(with: ["brew", "uninstall"]) })
  }

  /// 대상의 앱 번들 중 실행 중인 것(앱 이름). 번들 목록이 비었으면 휴지통 목록의 .app으로
  func runningApp(_ t: PlanTarget) async -> String? {
    let bundles = t.appBundles.isEmpty ? t.trash.map(\.path).filter { $0.hasSuffix(".app") } : t.appBundles
    for b in bundles { if let app = await runningOwner(b) { return app } }
    return nil
  }

  /// 마지막 관문 — 계획에 들어 있어도 여기서 한 번 더 막는다.
  func refuse(_ path: String) -> String? {
    let p = (path as NSString).standardizingPath
    if p.isEmpty || p == "/" || !p.hasPrefix("/") { return "경로가 이상합니다 — 옮기지 않았습니다" }
    if Paths.isProtected(p) { return "시스템 보호 경로 — 옮기지 않았습니다" }
    if me.isSelf(path: p) { return "MacInv 자신의 것 — 옮기지 않았습니다" }
    if Self.wholeFolders(home: me.home).contains(p.lowercased()) || Self.isVolumeRoot(p) { return "통째로 옮기면 안 되는 폴더 — 옮기지 않았습니다" }
    return nil
  }

  /// 통째로 옮기면 안 되는 폴더(소문자 — APFS는 기본으로 대소문자를 가리지 않아 `/applications`도 같은 곳).
  /// 지금 계획에서 이런 경로가 나오지는 않지만 안전장치로 둔다(2026-11-21 점검에서 보강).
  static func wholeFolders(home: String) -> Set<String> {
    let h = (home as NSString).standardizingPath
    let inHome = ["", "/Library", "/Library/Application Support", "/Library/Caches", "/Library/Preferences",
                  "/Library/Containers", "/Library/Group Containers", "/Library/LaunchAgents", "/Library/Logs",
                  "/Documents", "/Desktop", "/Downloads", "/Pictures", "/Movies", "/Music", "/Applications", "/.Trash"]
    return Set((inHome.map { h + $0 } + ["/Applications", "/Library", "/Users", "/Volumes", "/private", "/opt", "/opt/homebrew"]).map { $0.lowercased() })
  }

  /// `/Volumes/<디스크>` 자체(외장 디스크 전체)
  static func isVolumeRoot(_ p: String) -> Bool {
    let parts = p.split(separator: "/")
    return parts.count == 2 && parts[0].lowercased() == "volumes"
  }

  static func message(_ e: Error) -> String {
    if let c = e as? CommandError {
      if case .notFound(let n) = c { return "\(n)을(를) 찾지 못했습니다(설치돼 있지 않거나 PATH에 없음)" }
      return c.description
    }
    let ns = e as NSError
    if ns.domain == NSCocoaErrorDomain && (ns.code == NSFileWriteNoPermissionError || ns.code == 513) {
      return "권한이 없어 옮기지 못했습니다 — 시스템 설정에서 MacInv에 권한을 주거나 Finder에서 직접 옮기세요"
    }
    return ns.localizedDescription
  }
}

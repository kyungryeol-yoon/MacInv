import Foundation

/// 설치된 앱 하나(Bundle ID + 이름) — 다른 앱 소유 판정·벤더 판단에 쓴다.
public struct InstalledApp: Sendable, Hashable {
  public let id: String
  public let name: String
  public init(id: String, name: String) { self.id = id; self.name = name }
}

extension Paths {
  static let leftoverSuffixes = [".plist", ".savedState", ".binarycookies"]

  /// ByHost plist 이름의 호스트 UUID 접미사(`com.x.y.2CD92239-….plist`)를 뗀다.
  static func stripHostUUID(_ name: String) -> String {
    guard let r = name.range(of: #"\.[0-9A-Fa-f]{8}-[0-9A-Fa-f-]{27}$"#, options: .regularExpression) else { return name }
    return String(name[..<r.lowerBound])
  }

  /// 잔여물 경로에서 Bundle ID 후보를 뽑는다(순수): 확장자·ByHost UUID·팀 ID·`group.`을 뗀다.
  static func derivedBundleID(_ path: String) -> String {
    var name = (path as NSString).lastPathComponent
    if let s = leftoverSuffixes.first(where: { name.hasSuffix($0) }) { name.removeLast(s.count) }
    name = stripTeamIDPrefix(stripHostUUID(name))
    return name.hasPrefix("group.") ? String(name.dropFirst(6)) : name
  }
}

/// 삭제 계획을 만든다 — **읽기 전용(아무것도 지우지 않는다).** 입력은 인벤토리와 주입한 파일 시스템뿐이라
/// 안전 판정을 전부 테이블 테스트로 고정할 수 있다.
public enum Planner {
  public struct Env: Sendable {
    public var home: String
    public var fs: FileSystem
    /// 휴지통으로 옮길 수 있나 — 못 옮기면 이유(관리자 / macOS 권한)
    public var access: @Sendable (String) -> TrashAccess
    public var me: SelfIdentity
    /// 실행 중인 앱 이름(nil = 조회 실패)
    public var running: [String]?
    public init(home: String, fs: FileSystem = .real, access: @escaping @Sendable (String) -> TrashAccess = Planner.trashAccess,
                me: SelfIdentity, running: [String]? = []) {
      self.home = home; self.fs = fs; self.access = access; self.me = me; self.running = running
    }
  }

  static let selfRefusal = "MacInv 자신은 여기서 지우지 않습니다 — 지우려면 터미널에서 brew uninstall --cask macinv"

  // MARK: 앱

  public static func removal(app: AppItem, inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: "\(app.name) 삭제")
    if env.me.isSelf(path: app.path) || app.bundleID.map(env.me.isSelf(bundleID:)) == true { plan.refusals = [selfRefusal]; return plan }
    if Paths.isProtected(app.path) { plan.refusals = ["시스템 경로(\(app.path))라 건드리지 않습니다."]; return plan }
    if app.origin == .apple || Paths.isAppleSystemApp(bundleID: app.bundleID) {
      plan.refusals = ["애플 기본 앱이라 지우지 않습니다 — macOS에 딸린 앱이라 지우면 시스템 기능이 깨질 수 있습니다."]
      return plan
    }
    if case .brewCask(let token) = app.origin {
      var p = removal(cask: BrewCask(token: token, version: nil, appNames: []), inventory: inv, env: env)
      p.title = "\(app.name) 삭제"
      return p
    }
    var t = PlanTarget(kind: .app, name: app.name, bundleID: app.bundleID)
    t.trash = [PlanPath(PathMatch(path: app.path, confidence: .high, reason: "앱 번들"))]
    // 같은 Bundle ID의 다른 사본(다른 위치)
    if let id = app.bundleID {
      for other in inv.apps where other.bundleID == id && other.path != app.path && !Paths.isProtected(other.path) {
        t.trash.append(PlanPath(PathMatch(path: other.path, confidence: .high, reason: "같은 Bundle ID 앱 사본")))
      }
    }
    if let r = env.running, r.contains(app.name) {
      t.warnings.append("\(app.name)이(가) 실행 중입니다 — 지우기 전에 종료해야 합니다.")
    } else if env.running == nil {
      t.warnings.append("실행 중인 앱을 확인하지 못했습니다 — 지우기 전에 \(app.name)을(를) 종료했는지 확인하세요.")
    }
    if app.origin == .appStore { t.suggestions.append("App Store 앱입니다 — 지워도 구입 기록은 남아 App Store에서 다시 받을 수 있습니다.") }
    addData(&t, appName: app.name, inv: inv, env: env)
    plan.targets = [t]
    plan.requireConfirm = true
    return finish(plan, env: env)
  }

  // MARK: Homebrew

  public static func removal(cask: BrewCask, inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: "\(cask.token) 삭제")
    if env.me.isSelf(caskToken: cask.token) { plan.refusals = [selfRefusal]; return plan }
    var t = PlanTarget(kind: .brewCask, name: cask.token)
    t.commands = [["brew", "uninstall", "--cask", cask.token]]
    for app in inv.apps where app.origin == .brewCask(cask.token) {
      t.bundleID = t.bundleID ?? app.bundleID
      addData(&t, appName: app.name, bundleID: app.bundleID, inv: inv, env: env)
    }
    t.suggestions.append("앱 번들은 brew가 지웁니다. 남은 설정·캐시는 아래 경로를 휴지통으로 옮깁니다.")
    t.suggestions.append("끝나면 남은 의존성 정리: brew autoremove")
    plan.targets = [t]
    plan.requireConfirm = true
    return finish(plan, env: env)
  }

  public static func removal(formula f: BrewFormula, inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: "\(f.name) 삭제")
    var t = PlanTarget(kind: .brewFormula, name: f.name)
    t.commands = [["brew", "uninstall", f.name]]
    if let users = BrewGraph.dependents(inv.formulae)[f.name], !users.isEmpty {
      t.warnings.append("이걸 쓰는 포뮬러가 있습니다: \(users.joined(separator: ", ")) — brew가 지우기를 거부합니다. 먼저 그것들을 지우세요.")
    }
    for rt in inv.runtimes where rt.installed {
      guard let i = rt.origins.firstIndex(where: { $0.manager == .brew && $0.package == f.name }) else { continue }
      t.warnings.append(i == 0 ? "지금 쓰이는 \(rt.name) 런타임입니다 — 이걸 쓰는 프로젝트·도구가 동작하지 않게 될 수 있습니다."
                               : "\(rt.name) 런타임(지금 쓰이는 것은 아님)입니다 — 이걸 고정해 쓰는 프로젝트가 있는지 확인하세요.")
    }
    if !f.onRequest { t.suggestions.append("의존성으로 깔린 것입니다 — 하나씩 지우기보다 brew autoremove가 안전합니다.") }
    t.suggestions.append("끝나면 남은 의존성 정리: brew autoremove")
    plan.targets = [t]
    return plan
  }

  // MARK: 패키지

  public static func removal(package p: PackageItem, inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: "\(p.name) 삭제")
    var t: PlanTarget
    switch p.manager {
    case .npm:
      if p.name == "npm" { plan.refusals = ["npm 자신은 지우지 않습니다 — node와 함께 관리됩니다."]; return plan }
      t = PlanTarget(kind: .npm, name: p.name)
      t.commands = [["npm", "uninstall", "-g", p.name]]
    case .pipx:
      t = PlanTarget(kind: .pipx, name: p.name)
      t.commands = [["pipx", "uninstall", p.name]]
    case .cargo:
      t = PlanTarget(kind: .cargo, name: p.name)
      t.commands = [["cargo", "uninstall", p.name]]
    case .go:
      t = PlanTarget(kind: .goBin, name: p.name)
      let path = p.path ?? "\(env.home)/go/bin/\(p.name)"
      t.trash = [PlanPath(PathMatch(path: path, confidence: .high, reason: "go install로 깐 실행 파일"))]
      t.suggestions.append("go에는 삭제 명령이 없어 실행 파일을 휴지통으로 옮깁니다.")
    case .mas:
      if let app = inv.apps.first(where: { $0.origin == .appStore && $0.name == p.name }) {
        return removal(app: app, inventory: inv, env: env)
      }
      plan.refusals = ["앱 목록에서 \(p.name)을 찾지 못했습니다 — 앱 목록에서 지우세요."]
      return plan
    }
    plan.targets = [t]
    return finish(plan, env: env)
  }

  // MARK: 앱 데이터(잔여물 + 벤더 폴더)

  static func installed(_ inv: Inventory) -> [InstalledApp] {
    inv.apps.compactMap { a in a.bundleID.map { InstalledApp(id: $0, name: a.name) } }
  }

  static func addData(_ t: inout PlanTarget, appName: String, bundleID: String? = nil, inv: Inventory, env: Env) {
    let id = bundleID ?? t.bundleID
    let all = installed(inv)
    let (kept, excluded) = excludeOtherApps(Paths.leftovers(home: env.home, bundleID: id, appName: appName, fs: env.fs), selfID: id, installed: all)
    t.excluded += excluded
    for m in kept where !t.trash.contains(where: { $0.path == m.path }) { t.trash.append(PlanPath(m)) }
    // 다른 설치 앱에서 파생된 앱(Chrome 웹 앱 com.google.Chrome.app.…)은 벤더 앱이 아니다 — 벤더 폴더를 건드리지 않는다
    if let id, owner(of: id, installed: all.filter { $0.id != id }) == nil {
      let v = vendor(home: env.home, bundleID: id, appName: appName, installedIDs: all.map(\.id), existing: t.trash.map(\.path), fs: env.fs)
      t.trash += v.trash.map { PlanPath($0) }
      t.excluded += v.excluded
    }
  }

  /// 파생 ID를 소유하는 설치 앱 — **가장 긴 ID가 이긴다**(Chrome과 Canary가 둘 다 있을 때 Canary 파생물이 Chrome 것으로 오판되면 안 됨).
  static func owner(of candidate: String, installed: [InstalledApp]) -> InstalledApp? {
    installed.filter { !$0.id.isEmpty && (candidate == $0.id || candidate.hasPrefix($0.id + ".")) }.max { $0.id.count < $1.id.count }
  }

  /// 접두사가 삼킨 다른 설치 앱 소유 경로를 빼고 이유를 남긴다(Go판 6-1, 데이터 손실 직전).
  /// **어느 앱도 소유하지 않는 파생물(`.helper` 등)은 남겨서 지운다** — 이 구분이 핵심.
  static func excludeOtherApps(_ matches: [PathMatch], selfID: String?, installed: [InstalledApp]) -> ([PathMatch], [Exclusion]) {
    guard let selfID, !selfID.isEmpty else { return (matches, []) }
    var kept: [PathMatch] = [], excluded: [Exclusion] = []
    for m in matches {
      if let o = owner(of: Paths.derivedBundleID(m.path), installed: installed), o.id != selfID {
        excluded.append(Exclusion(path: m.path, reason: "다른 앱(\(o.name)) 것 — 보존"))
      } else {
        kept.append(m)
      }
    }
    return (kept, excluded)
  }

  /// 벤더 폴더(Google·Microsoft…): 같은 벤더 앱이 더 있으면 `<Vendor>/<App>`만 [확신낮음] + 공유 컴포넌트는 보존,
  /// 이 앱이 마지막이면 벤더 폴더 전체 + 공유 컴포넌트를 [확신중간].
  static func vendor(home: String, bundleID: String, appName: String, installedIDs: [String], existing: [String], fs: FileSystem)
    -> (trash: [PathMatch], excluded: [Exclusion]) {
    guard let (v, sub) = Vendors.lookup(bundleID: bundleID, appName: appName) else { return ([], []) }
    var seen = Set(existing), trash: [PathMatch] = [], excluded: [Exclusion] = []
    func covered(_ p: String) -> Bool { (existing + trash.map(\.path)).contains { p == $0 || p.hasPrefix($0 + "/") } }
    // 이 앱에서 파생된 앱(Chrome 웹 앱 등)은 "같은 벤더의 다른 앱"으로 세지 않는다 — 이 앱 없이는 못 돈다
    let others = installedIDs.filter { $0 != bundleID && !$0.hasPrefix(bundleID + ".") && v.bundle_prefixes.contains(where: $0.hasPrefix) }.count
    let roots = Paths.libraryRoots(home: home)
    if others > 0 {
      for root in roots { for folder in v.folders {
        let p = "\(root)/\(folder)/\(sub)"
        guard !sub.isEmpty, !Paths.isProtected(p), fs.exists(p), seen.insert(p).inserted else { continue }
        trash.append(PathMatch(path: p, confidence: .low, reason: "\(v.vendor) 폴더 안 이 앱 폴더(확신 낮음)"))
      } }
      for sc in v.shared_components { for g in sc.globs { for p in expand(home: home, glob: g, fs: fs) where !Paths.isProtected(p) {
        excluded.append(Exclusion(path: p, reason: "다른 \(v.vendor) 앱이 씀 — 보존(\(sc.name))"))
      } } }
      return (trash, excluded)
    }
    for root in roots { for folder in v.folders {
      let p = "\(root)/\(folder)"
      guard !Paths.isProtected(p), fs.exists(p), seen.insert(p).inserted else { continue }
      trash.append(PathMatch(path: p, confidence: .medium, reason: "\(v.vendor) 폴더 전체(이 벤더 앱은 이것뿐)"))
    } }
    for sc in v.shared_components { for g in sc.globs { for p in expand(home: home, glob: g, fs: fs) {
      guard !Paths.isProtected(p), !covered(p), seen.insert(p).inserted else { continue }
      trash.append(PathMatch(path: p, confidence: .medium, reason: "\(v.vendor) 공유 컴포넌트(이 벤더 앱은 이것뿐 · \(sc.name))"))
    } } }
    return (trash, excluded)
  }

  /// 벤더 표의 홈 기준 경로. 끝의 `*`만 접두사 뜻(패턴 해석 없음).
  static func expand(home: String, glob: String, fs: FileSystem) -> [String] {
    let full = "\(home)/\(glob)"
    guard full.hasSuffix("*") else { return fs.exists(full) ? [full] : [] }
    let dir = (full as NSString).deletingLastPathComponent
    let prefix = String((full as NSString).lastPathComponent.dropLast())
    return fs.list(dir).filter { $0.hasPrefix(prefix) }.sorted().map { "\(dir)/\($0)" }
  }

  // MARK: 마무리

  /// 모든 계획의 마지막 관문: 보호 경로·MacInv 자신의 것은 다시 걸러 빼고, 권한이 없어 못 옮길 경로는 따로 둔다.
  static func finish(_ plan: Plan, env: Env) -> Plan {
    var p = plan
    for i in p.targets.indices {
      var keep: [PlanPath] = []
      for x in p.targets[i].trash {
        if Paths.isProtected(x.path) { continue }
        if env.me.isSelf(path: x.path) { p.targets[i].excluded.append(Exclusion(path: x.path, reason: "MacInv 자신의 것 — 보존")); continue }
        switch env.access(x.path) {
        case .ok: keep.append(x)
        case .needsAdmin: p.targets[i].needsAdmin.append(x)
        case .needsPermission: p.targets[i].needsPermission.append(x)
        }
      }
      p.targets[i].trash = Paths.dedupContainedPlan(keep)
      if !p.targets[i].needsAdmin.isEmpty {
        p.targets[i].suggestions.append("🔑 다른 사용자(root) 소유라 자동으로 옮기지 않습니다 — Finder에서 직접 휴지통으로 옮기세요(관리자 암호를 묻습니다).")
      }
    }
    return p
  }

  /// 부모 폴더 쓰기 권한(휴지통 이동은 부모에서 빼내는 일) + 폴더면 자기 쓰기 권한(폴더를 옮기면 그 안의 `..`가 바뀐다 —
  /// App Store 앱처럼 root 소유 번들은 /Applications가 쓰기 가능해도 못 옮긴다).
  /// 내 것인데 막히면 소유권 문제가 아니라 macOS 보호다(2026-11-21 실측: 내 소유 Claude.app·Chrome 데이터 폴더가 쓰기 불가 —
  /// 앱 관리·앱 데이터 보호). 관리자 암호로는 안 풀리고 시스템 설정에서 MacInv에 권한을 줘야 한다.
  @Sendable public static func trashAccess(_ path: String) -> TrashAccess {
    func check(_ p: String) -> TrashAccess {
      if Darwin.access(p, W_OK) == 0 { return .ok }
      var st = stat()
      return lstat(p, &st) == 0 && st.st_uid == getuid() ? .needsPermission : .needsAdmin
    }
    let parent = check((path as NSString).deletingLastPathComponent)
    guard parent == .ok else { return parent }
    var dir: ObjCBool = false
    if FileManager.default.fileExists(atPath: path, isDirectory: &dir), dir.boolValue { return check(path) }
    return .ok
  }

  /// 담긴 경로를 실제로 잰다(계획에 크기를 채운다).
  public static func measure(_ plan: Plan, runner: CommandRunner) async -> Plan {
    let all = plan.targets.flatMap { $0.trash + $0.needsAdmin + $0.needsPermission }.map(\.path)
    let sizes = await DiskUsage.measure(all, runner: runner)
    var p = plan
    for i in p.targets.indices {
      for j in p.targets[i].trash.indices { p.targets[i].trash[j].sizeKB = sizes[p.targets[i].trash[j].path] }
      for j in p.targets[i].needsAdmin.indices { p.targets[i].needsAdmin[j].sizeKB = sizes[p.targets[i].needsAdmin[j].path] }
      for j in p.targets[i].needsPermission.indices { p.targets[i].needsPermission[j].sizeKB = sizes[p.targets[i].needsPermission[j].path] }
    }
    return p
  }
}

extension Paths {
  /// 계획 경로의 부모·자식 중복 제거. 넓은 쪽이 **확신도가 같거나 높을 때만** 좁은 쪽을 삼킨다 —
  /// [확신낮음] 부모가 [확실] 자식을 삼키면, 낮음을 안 골랐을 때 확실한 것까지 빠진다.
  static func dedupContainedPlan(_ input: [PlanPath]) -> [PlanPath] {
    var out: [PlanPath] = []
    for m in input {
      if out.contains(where: { $0.path == m.path || (m.path.hasPrefix($0.path + "/") && $0.confidence >= m.confidence) }) { continue }
      out.removeAll { $0.path.hasPrefix(m.path + "/") && m.confidence >= $0.confidence }
      out.append(m)
    }
    return out
  }
}

import Foundation

// 5-2: 업데이트 · 고아 잔여물 · 디스크 정리 · Ollama 모델 계획. 5-1과 같은 Plan·마지막 관문(finish)을 쓴다.

extension Planner {
  static let upgradeTimeout: TimeInterval = 1800
  static let cleanupTimeout: TimeInterval = 900

  // MARK: 업데이트

  /// 업데이트 계획(순수). `everything`이면 "전부 업데이트" — 고정된 것·**런타임 메이저**·sudo 필요 cask는 빼고 이유와 명령을 남긴다.
  /// 골라서 하는 것(`everything == false`)은 런타임 메이저도 넣되 경고한다. MacInv 자신은 **맨 마지막**에, 재시작 안내와 함께.
  public static func upgrade(_ items: [OutdatedItem], everything: Bool, inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: everything ? "모두 업데이트" : (items.count == 1 ? "\(items[0].name) 업데이트" : "\(items.count)개 업데이트"))
    plan.verb = "업데이트"
    var selfTarget: PlanTarget?
    let installers = Set(inv.casks.filter(\.usesInstaller).map(\.token))
    for item in items {
      let cmd = upgradeCommand(item)
      if item.pinned {
        plan.held.append(HeldItem(name: item.name, reason: "brew pin으로 고정함 — 풀려면 brew unpin \(item.name)", command: nil)); continue
      }
      if everything && item.runtimeMajor {
        plan.held.append(HeldItem(name: item.name, reason: "런타임 메이저(\(item.current) → \(item.latest)) — 프로젝트가 깨질 수 있어 「모두 업데이트」에서 뺍니다", command: cmd)); continue
      }
      if item.source == .brewCask && installers.contains(item.name) {
        plan.held.append(HeldItem(name: item.name, reason: "설치기(pkg)를 써서 관리자 암호가 필요합니다 — 터미널에서 직접 실행하세요", command: cmd)); continue
      }
      if item.source == .mas && cmd == nil {
        plan.held.append(HeldItem(name: item.name, reason: "App Store 앱 ID를 몰라 명령을 만들 수 없습니다 — App Store에서 업데이트하세요", command: nil)); continue
      }
      var t = PlanTarget(kind: kind(item.source), name: item.name)
      t.commands = cmd.map { [$0] } ?? []
      t.timeout = upgradeTimeout
      t.suggestions.append("\(item.current) → \(item.latest)")
      if item.runtimeMajor { t.warnings.append("런타임 메이저 업그레이드입니다(\(item.current) → \(item.latest)) — 이걸 쓰는 프로젝트가 깨질 수 있습니다. 런타임 화면에서 지원 기간을 확인하세요.") }
      if item.name == "npm" && item.source == .npm { t.suggestions.append("npm 자신은 npx로 올립니다 — 실행 중인 npm이 자기 파일을 지우다 멈추는 것을 피하려고") }
      if item.source == .brewCask || item.source == .mas, let r = env.running,
         let app = inv.apps.first(where: { appMatches($0, item) }), r.contains(app.name) {
        t.warnings.append("\(app.name)이(가) 실행 중입니다 — 업데이트하면서 종료될 수 있습니다.")
      }
      if item.source == .brewCask && env.me.isSelf(caskToken: item.name) {
        t.suggestions.append("MacInv 자신입니다 — 맨 마지막에 하고, 끝나면 MacInv를 다시 시작해야 합니다. 권한(전체 디스크 접근 등)을 다시 켜야 할 수 있습니다.")
        selfTarget = t; continue
      }
      plan.targets.append(t)
    }
    if let selfTarget { plan.targets.append(selfTarget) }
    return plan
  }

  static func kind(_ s: OutdatedItem.Source) -> TargetKind {
    switch s { case .brewFormula: return .brewFormula; case .brewCask: return .brewCask; case .npm: return .npm; case .mas: return .mas }
  }

  /// 명령(인자 배열). App Store 앱은 ID가 있어야 한다.
  static func upgradeCommand(_ i: OutdatedItem) -> [String]? {
    switch i.source {
    case .brewFormula: return ["brew", "upgrade", i.name]
    case .brewCask: return ["brew", "upgrade", "--cask", i.name]
    case .npm: return i.name == "npm" ? ["npx", "-y", "npm@latest", "install", "-g", "npm@latest"] : ["npm", "install", "-g", "\(i.name)@latest"]
    case .mas: return i.masID.map { ["mas", "upgrade", $0] }
    }
  }

  static func appMatches(_ a: AppItem, _ i: OutdatedItem) -> Bool {
    i.source == .brewCask ? a.origin == .brewCask(i.name) : (a.origin == .appStore && a.name == i.name)
  }

  // MARK: 고아 잔여물(지운 앱이 ~/Library에 남긴 것)

  /// ~/Library만 본다(/Library는 다른 사용자·서비스 소유일 수 있다). strip이 있으면 그 접미사를 가진 것만.
  static let orphanLocations: [(rel: String, strip: String, reason: String)] = [
    ("Preferences", ".plist", "환경설정"), ("Preferences/ByHost", ".plist", "환경설정 ByHost"), ("Caches", "", "캐시"),
    ("HTTPStorages", "", "HTTP 저장소"), ("WebKit", "", "WebKit 데이터"), ("Containers", "", "샌드박스 컨테이너"),
    ("Application Scripts", "", "앱 스크립트"), ("Saved Application State", ".savedState", "저장된 상태"),
    ("Application Support", "", "앱 지원 데이터"), ("Logs", "", "로그"), ("LaunchAgents", ".plist", "LaunchAgent"),
    ("Cookies", ".binarycookies", "쿠키"), ("Group Containers", "", "그룹 컨테이너"),
  ]

  /// 항목 이름에서 Bundle ID를 뽑는다(순수). 접미사가 필요한데 없으면 nil.
  static func orphanID(_ name: String, strip: String) -> String? {
    var n = name
    if !strip.isEmpty { guard n.hasSuffix(strip) else { return nil }; n.removeLast(strip.count) }
    n = Paths.stripHostUUID(n)
    return n.hasPrefix("group.") ? String(n.dropFirst(6)) : n
  }

  /// reverse-DNS 꼴(점 2개 이상). "Google"·"Notion" 같은 이름 폴더는 귀속을 확신할 수 없어 아예 대상이 아니다.
  static func bundleIDShaped(_ id: String) -> Bool {
    id.range(of: #"^[A-Za-z0-9][A-Za-z0-9-]*(\.[A-Za-z0-9_-]+){2,}$"#, options: .regularExpression) != nil
  }

  /// 무조건 보존(실기 오탐 — Apple 팟캐스트 그룹·단축어·Sparkle): `com.apple.`이 어디든, `systemgroup.`, 팀 ID 접두사,
  /// `is.workflow.`(단축어), `org.sparkle-project.`(앱들이 넣어 쓰는 업데이트 프레임워크)
  static func orphanProtected(_ id: String) -> Bool {
    id.contains("com.apple.") || id.hasPrefix("systemgroup.") || Paths.stripTeamIDPrefix(id) != id
      || ["is.workflow.", "org.sparkle-project."].contains(where: id.hasPrefix)
  }

  /// 고아인가(순수). **애매하면 고아가 아니다.** 대소문자는 무시한다(Bundle ID는 대소문자를 가리지 않는다).
  /// - 설치 ID와 **양방향** 접두사 관계면 보존(파생물·상위 ID 모두)
  /// - 벤더 접두사에 맞고 그 벤더 앱이 하나라도 있으면 보존(공유 컴포넌트일 수 있음)
  /// - ID 마디 하나가 설치된 도구 이름이면 보존(`org.swift.swiftpm`←swift, `sh.brew.ollama`·`homebrew.mxcl.redis`←포뮬러 — 2026-11-21 실측)
  /// - 설치 앱과 **같은 개발자**(ID 앞 두 마디, `io.github`처럼 흔한 것은 세 마디)면 보존 — `com.logi.cp-dev-mgr`(Logi Options+ 설치됨) 실측
  static func isOrphan(_ id: String, installed: [String], tools: Set<String>, vendors: [Vendor] = Vendors.all) -> Bool {
    guard bundleIDShaped(id), !orphanProtected(id) else { return false }
    let l = id.lowercased()
    if l.split(separator: ".").dropFirst().contains(where: { tools.contains(String($0)) }) { return false }
    let mine = developer(l)
    if installed.contains(where: { developer($0.lowercased()) == mine }) { return false }
    for i in installed.map({ $0.lowercased() }) where !i.isEmpty {
      if l == i || l.hasPrefix(i + ".") || i.hasPrefix(l + ".") { return false }
    }
    for v in vendors where v.bundle_prefixes.contains(where: { l.hasPrefix($0.lowercased()) }) {
      if installed.contains(where: { i in v.bundle_prefixes.contains { i.lowercased().hasPrefix($0.lowercased()) } }) { return false }
    }
    return true
  }

  /// 개발자 접두사: 앞 두 마디(`com.logi`). `io.github` 같은 공용 호스트는 세 마디(`io.github.kyungryeol-yoon`).
  static func developer(_ id: String) -> String {
    let parts = id.split(separator: ".")
    let shared: Set<String> = ["io.github", "com.github", "io.gitlab", "com.gitlab", "net.sourceforge", "org.sourceforge", "com.googlecode", "io.bitbucket"]
    let n = shared.contains(parts.prefix(2).joined(separator: ".")) ? 3 : 2
    return parts.prefix(n).joined(separator: ".")
  }

  /// 고아 잔여물 계획 — ID별로 묶는다. 전부 [확신중간]("설치 목록에 없음"은 추론 — 앱 없는 CLI가 plist를 쓰기도 한다).
  public static func orphans(inventory inv: Inventory, env: Env) -> Plan {
    var plan = Plan(title: "고아 잔여물 정리")
    plan.verb = "정리"
    let installed = inv.apps.compactMap(\.bundleID) + [env.me.bundleID]
    // 앱 없이 도는 도구: 포뮬러·패키지·런타임 이름
    let tools = Set((inv.formulae.map(\.name) + inv.packages.map(\.name) + inv.runtimes.filter(\.installed).map(\.name)).map { $0.lowercased() })
    let lib = "\(env.home)/Library"
    var byID: [String: [PathMatch]] = [:]
    for loc in orphanLocations {
      let dir = "\(lib)/\(loc.rel)"
      for name in env.fs.list(dir).sorted() {
        guard let id = orphanID(name, strip: loc.strip), isOrphan(id, installed: installed, tools: tools) else { continue }
        byID[id.lowercased(), default: []].append(PathMatch(path: "\(dir)/\(name)", confidence: .medium, reason: "\(loc.reason) — 설치된 앱 중 이 ID가 없음"))
      }
    }
    // 파생 ID(`org.m0k.transmission.quicklookextension`)는 상위 고아(`org.m0k.transmission`) 아래로 — 한 앱이 한 줄
    for id in byID.keys.sorted(by: { $0.count > $1.count }) {
      if let parent = byID.keys.filter({ id.hasPrefix($0 + ".") }).min(by: { $0.count < $1.count }) {
        byID[parent]! += byID[id]!; byID[id] = nil
      }
    }
    for id in byID.keys.sorted() {
      var t = PlanTarget(kind: .orphan, name: id, bundleID: id)
      t.trash = byID[id]!.map { PlanPath($0) }
      plan.targets.append(t)
    }
    if !plan.targets.isEmpty {
      plan.warnings.append("설치된 앱 목록에 없는 ID라 지운 앱의 것으로 보입니다. 다만 앱 없이 도는 도구가 남긴 것일 수 있으니 모르는 이름은 빼고 진행하세요.")
      plan.requireConfirm = true
    } else {
      plan.refusals = ["고아 잔여물이 없습니다."]
    }
    return finish(plan, env: env)
  }

  // MARK: 디스크 정리

  /// 디스크 정리 계획(순수). `only`가 있으면 그 위치만. never는 어떤 옵션으로도 안 들어가고, expensive는 `includeExpensive`일 때만.
  /// **실행 중인 앱의 캐시는 언제나 뺀다**(UI에 끄는 수단 없음). 조회가 실패했는데 그 보호가 필요한 위치를 담았으면 계획 맨 위에 경고.
  public static func diskCleanup(_ report: DiskReport, only: Set<String>? = nil, includeExpensive: Bool, env: Env) -> Plan {
    var plan = Plan(title: only?.count == 1 ? "\(report.entries.first { only!.contains($0.id) }?.location.name ?? "") 정리" : "디스크 정리")
    plan.verb = "정리"
    var unverified = false
    for e in report.entries {
      if let only, !only.contains(e.id) { continue }
      let loc = e.location
      guard loc.tier.cleanable(includeExpensive: includeExpensive) else {
        if only != nil { plan.refusals.append(loc.tier == .never ? "\(loc.name) — 정리하지 않는 곳입니다(소스·사용자 데이터)." : "\(loc.name) — 재다운로드가 비싼 곳이라 「비싼 것 포함」을 켜야 들어갑니다.") }
        continue
      }
      var t = PlanTarget(kind: .disk, name: "\(loc.name) · \(loc.tier.label)")
      t.timeout = cleanupTimeout
      // 고아 캐시: 쓰는 도구가 없으니 그 도구의 정리 명령도 못 돈다 → 폴더를 휴지통으로
      let useTrash = loc.trash == true || (e.orphan && !(loc.cleanup ?? []).isEmpty)
      if !useTrash, let c = loc.cleanup, !c.isEmpty {
        t.commands = c
        plan.extraReclaimKB += e.sizeKB    // 명령이 비울 몫 — 안 더하면 회수 예상이 실제보다 훨씬 작다
      } else if useTrash {
        for p in e.paths {
          if let app = e.blockedByRunning[p] { t.excluded.append(Exclusion(path: p, reason: "\(app) 실행 중 — 앱을 끈 뒤 다시 스캔하세요")); continue }
          if let target = e.linkedTo[p] { t.excluded.append(Exclusion(path: p, reason: "바로가기(→ \(target)) — 실제 위치는 따로 정리하세요")); continue }
          t.trash.append(PlanPath(PathMatch(path: p, confidence: e.orphan ? .medium : .high, reason: e.orphan ? "고아 캐시 — 쓰는 도구가 없음" : "다시 만들어지는 캐시"), sizeKB: e.pathSizes[p]))
        }
      } else {
        if only != nil { plan.refusals.append("\(loc.name) — \(loc.note ?? "정리 방법이 없습니다.")") }
        continue
      }
      if e.orphan { t.warnings.append("고아 캐시 — 「\(loc.tool)」을(를) 쓰는 도구·앱이 이 맥에 없습니다(지운 뒤 남은 것으로 보입니다).") }
      if let n = loc.note { t.suggestions.append(n) }
      if loc.tier == .expensive { plan.requireConfirm = true }
      if report.runningCheckFailed && loc.group_by == Disk.groupByAppSupportDir { unverified = true }
      if t.commands.isEmpty && t.trash.isEmpty && t.excluded.isEmpty { continue }
      plan.targets.append(t)
    }
    if unverified {
      plan.warnings.append("실행 중인 앱을 확인하지 못했습니다 — 켜져 있는 앱의 캐시를 지우면 오작동할 수 있으니 앱을 모두 끈 뒤 진행하세요.")
    }
    if plan.targets.isEmpty && plan.refusals.isEmpty { plan.refusals = ["지금 정리할 것이 없습니다."] }
    return finish(plan, env: env)
  }

  // MARK: Ollama 모델

  /// 모델별 삭제(`ollama rm`). 위치 통째로 지우지 않는다. 모델이 외장 디스크에 있으면 내장 디스크 여유는 늘지 않는다.
  public static func ollama(_ models: [OllamaModel], externalVolume: String?) -> Plan {
    var plan = Plan(title: models.count == 1 ? "\(models[0].name) 삭제" : "Ollama 모델 \(models.count)개 삭제")
    for m in models {
      var t = PlanTarget(kind: .ollama, name: m.name)
      t.commands = [["ollama", "rm", m.name]]
      t.suggestions.append("다시 쓰려면 ollama pull \(m.name) — \(m.sizeKB / 1_048_576 > 0 ? "\(m.sizeKB / 1_048_576) GB" : "\(m.sizeKB / 1024) MB")를 다시 받아야 합니다.")
      plan.targets.append(t)
      plan.extraReclaimKB += m.sizeKB
    }
    if let v = externalVolume { plan.warnings.append("모델이 외장 디스크 「\(v)」에 있습니다 — 지우면 그 디스크가 비고, 맥 내장 디스크 여유는 늘지 않습니다.") }
    plan.requireConfirm = true
    return plan
  }
}

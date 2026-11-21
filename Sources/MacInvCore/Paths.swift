import Foundation

/// 경로 판별·매칭 규칙 — **한 벌만 둔다.**
/// 용량 측정(2-3)과 삭제 계획(5단계)이 서로 다른 규칙을 갖는 순간 "보이는 용량"과 "지워지는 것"이 어긋난다.
///
/// Go판과 다른 점: glob을 쓰지 않고 폴더 목록을 읽어 문자열로 비교한다(접두사·접미사·포함).
/// 그래서 앱 이름·Bundle ID에 `[`·`*`·`?`가 있어도 패턴으로 해석될 일이 구조적으로 없다(Go판 6-2).
public enum Confidence: String, Sendable, Comparable {
  case high    // Bundle ID 정확 일치(접두사)
  case medium  // 벤더 폴더 전체(그 벤더 앱이 이것뿐일 때) — 5단계
  case low     // 이름·벤더 하위 폴더 기반 추정
  static let order: [Confidence] = [.low, .medium, .high]
  public static func < (a: Confidence, b: Confidence) -> Bool { order.firstIndex(of: a)! < order.firstIndex(of: b)! }
}

public struct PathMatch: Sendable, Hashable {
  public let path: String
  public let confidence: Confidence
  public let reason: String
}

/// 파일 시스템(테스트에서 가짜로 바꾼다).
public struct FileSystem: Sendable {
  public var list: @Sendable (String) -> [String]
  public var exists: @Sendable (String) -> Bool
  public init(list: @escaping @Sendable (String) -> [String], exists: @escaping @Sendable (String) -> Bool) {
    self.list = list; self.exists = exists
  }
  /// exists는 심볼릭 링크 자체도 있는 것으로 친다(따라가지 않음).
  public static let real = FileSystem(
    list: { (try? FileManager.default.contentsOfDirectory(atPath: $0)) ?? [] },
    exists: { (try? FileManager.default.attributesOfItem(atPath: $0)) != nil })
}

public enum Paths {
  /// 절대 접근 금지(하드코딩 블랙리스트). 삭제 직전에도 다시 확인한다.
  static let protectedPrefixes = ["/System", "/usr", "/bin", "/sbin", "/Library/Apple"]

  public static func isProtected(_ path: String) -> Bool {
    let p = (path as NSString).standardizingPath
    return protectedPrefixes.contains { p == $0 || p.hasPrefix($0 + "/") }
  }

  /// /Applications에 있어 경로 블랙리스트에 안 걸리는 애플 기본 앱(Safari 등).
  public static func isAppleSystemApp(bundleID: String?) -> Bool { bundleID?.hasPrefix("com.apple.") ?? false }

  // MARK: 잔여물(삭제 계획과 같은 규칙)

  /// Bundle ID로 찾는 위치: 폴더 + 이름 규칙. 모두 [확실].
  struct IDRule { let dir: String; let suffix: String; let contains: Bool; let reason: String }

  static func idRules(home: String) -> [IDRule] {
    let lib = "\(home)/Library"
    func r(_ sub: String, _ suffix: String = "", _ reason: String) -> IDRule { IDRule(dir: "\(lib)/\(sub)", suffix: suffix, contains: false, reason: reason) }
    func s(_ dir: String, _ suffix: String = "", _ reason: String) -> IDRule { IDRule(dir: dir, suffix: suffix, contains: false, reason: reason) }
    return [
      r("Preferences", ".plist", "환경설정"),
      r("Preferences/ByHost", ".plist", "환경설정 ByHost"),
      r("Caches", "", "캐시"),
      r("HTTPStorages", "", "HTTP 저장소"),
      r("WebKit", "", "WebKit 데이터"),
      r("Containers", "", "샌드박스 컨테이너"),
      r("Application Scripts", "", "앱 스크립트"),
      r("Saved Application State", ".savedState", "저장된 상태"),
      r("Application Support", "", "앱 지원 데이터"),
      r("Logs", "", "로그"),
      r("Cookies", ".binarycookies", "쿠키"),
      r("LaunchAgents", ".plist", "LaunchAgent"),
      IDRule(dir: "\(lib)/Group Containers", suffix: "", contains: true, reason: "그룹 컨테이너"),
      // 시스템 전역 — 지울 때 관리자 권한이 필요할 수 있다(5단계에서 분리)
      s("/Library/Application Support", "", "앱 지원(시스템)"),
      s("/Library/Caches", "", "캐시(시스템)"),
      s("/Library/Preferences", ".plist", "환경설정(시스템)"),
      s("/Library/LaunchAgents", ".plist", "LaunchAgent(시스템)"),
      s("/Library/LaunchDaemons", ".plist", "LaunchDaemon(시스템)"),
    ]
  }

  /// 이름이 Bundle ID 규칙에 맞는가(순수). 접두사 매칭이라 헬퍼·파생 파일(`<ID>.helper.plist`)까지 잡는다.
  /// 접두사가 다른 앱 ID까지 삼키는 문제(`com.google.Chrome` → `.canary`)는 5단계 계획에서 걷어낸다.
  static func matches(_ name: String, id: String, rule: IDRule) -> Bool {
    if rule.contains { return name.contains(id) }
    return name.hasPrefix(id) && (rule.suffix.isEmpty || name.hasSuffix(rule.suffix))
  }

  /// 이름으로 찾는 위치 — **리터럴 경로**(패턴 아님). [확신낮음].
  static func nameLiterals(home: String, appName: String) -> [(String, String)] {
    guard !appName.isEmpty, !appName.contains("/") else { return [] }
    let lib = "\(home)/Library"
    return [("\(lib)/Application Support/\(appName)", "앱 지원 데이터(이름 기반)"),
            ("\(lib)/Caches/\(appName)", "캐시(이름 기반)"),
            ("\(lib)/Logs/\(appName)", "로그(이름 기반)")]
  }

  /// 앱이 남긴 데이터(삭제 계획과 같은 규칙). Bundle ID 규칙이 먼저라 같은 경로면 [확실]이 이긴다.
  public static func leftovers(home: String, bundleID: String?, appName: String, fs: FileSystem = .real) -> [PathMatch] {
    var seen = Set<String>(), out: [PathMatch] = []
    func take(_ p: String, _ c: Confidence, _ reason: String) {
      guard !isProtected(p), seen.insert(p).inserted else { return }
      out.append(PathMatch(path: p, confidence: c, reason: reason))
    }
    if let id = bundleID, !id.isEmpty {
      for rule in idRules(home: home) {
        for name in fs.list(rule.dir).sorted() where matches(name, id: id, rule: rule) {
          take("\(rule.dir)/\(name)", .high, rule.reason)
        }
      }
    }
    for (p, reason) in nameLiterals(home: home, appName: appName) where fs.exists(p) { take(p, .low, reason) }
    return out
  }

  // MARK: 측정 전용(삭제에 쓰지 않는다)

  /// **용량 측정 전용** 경로. 잔여물 규칙에 두 가지를 더한다 — 이 앱 것일 가능성이 높다는 추정이라 지우지는 않는다.
  /// ① 벤더 폴더 안 앱 하위 폴더: Chrome 데이터는 `Application Support/Google/Chrome`(ID로도 이름으로도 안 잡힘)
  /// ② 그룹 컨테이너 이름 매칭: OrbStack은 ID가 `dev.kdrag0n.MacVirt`라 `HUAQ24HBR6.dev.orbstack`(24 GB)을 ID로는 못 찾는다
  public static func sizePaths(home: String, bundleID: String?, appName: String, fs: FileSystem = .real) -> [PathMatch] {
    var out = leftovers(home: home, bundleID: bundleID, appName: appName, fs: fs)
    func add(_ p: String, _ reason: String) {
      if !isProtected(p), fs.exists(p) { out.append(PathMatch(path: p, confidence: .low, reason: reason)) }
    }
    for p in vendorAppDirs(home: home, bundleID: bundleID, appName: appName) { add(p, "벤더 폴더 안 앱 데이터") }
    let gc = "\(home)/Library/Group Containers"
    for name in fs.list(gc).sorted() where groupContainerMatchesApp(name, appName: appName) { add("\(gc)/\(name)", "그룹 컨테이너(이름 기반)") }
    return dedupContained(out)
  }

  /// 부모·자식 경로가 함께 있으면 부모만 남긴다(순수). 함께 du에 넘기면 자식이 두 번 세진다(Go판 6-4).
  /// 양방향: 새 경로가 기존의 하위면 버리고, 부모면 기존 자식을 걷어낸다.
  public static func dedupContained(_ input: [PathMatch]) -> [PathMatch] {
    var out: [PathMatch] = []
    for m in input where !m.path.isEmpty {
      if out.contains(where: { $0.path == m.path || m.path.hasPrefix($0.path + "/") }) { continue }
      out.removeAll { $0.path.hasPrefix(m.path + "/") }
      out.append(m)
    }
    return out
  }

  /// 팀 ID 접두사(`HUAQ24HBR6.`)를 뗀다.
  public static func stripTeamIDPrefix(_ name: String) -> String {
    guard name.count > 11 else { return name }
    let head = name.prefix(10), dot = name[name.index(name.startIndex, offsetBy: 10)]
    return dot == "." && head.allSatisfy({ $0.isUppercase || $0.isNumber }) && head.allSatisfy(\.isASCII) ? String(name.dropFirst(11)) : name
  }

  /// 그룹 컨테이너 폴더가 이 앱 것인가(순수): 팀 ID를 떼고 **마지막 마디만** 앱 이름과 정확히 비교.
  /// 부분 일치 금지 — "Mail"·"Home" 같은 짧은 이름이 무관한 컨테이너를 쓸어 담는다(완화 금지).
  public static func groupContainerMatchesApp(_ folder: String, appName: String) -> Bool {
    let name = normalizeAppKey(appName)
    guard !name.isEmpty else { return false }
    let stripped = stripTeamIDPrefix(folder)
    guard let dot = stripped.lastIndex(of: ".") else { return false }   // 마디가 하나면 Bundle ID 꼴이 아님
    return normalizeAppKey(String(stripped[stripped.index(after: dot)...])) == name
  }

  static func normalizeAppKey(_ s: String) -> String {
    var t = s
    if t.lowercased().hasSuffix(".app") { t.removeLast(4) }
    return t.replacingOccurrences(of: " ", with: "").replacingOccurrences(of: "-", with: "").replacingOccurrences(of: "_", with: "").lowercased()
  }

  // MARK: 벤더 폴더

  /// 벤더 폴더가 있을 수 있는 Library 아래 위치.
  static func libraryRoots(home: String) -> [String] {
    let lib = "\(home)/Library"
    return ["\(lib)/Application Support", "\(lib)/Caches", "\(lib)/Logs", lib]
  }

  /// 벤더 폴더 안 이 앱 전용 하위 폴더(공유 컴포넌트 제외).
  static func vendorAppDirs(home: String, bundleID: String?, appName: String) -> [String] {
    guard let id = bundleID, let (v, sub) = Vendors.lookup(bundleID: id, appName: appName), !sub.isEmpty else { return [] }
    return libraryRoots(home: home).flatMap { root in v.folders.map { "\(root)/\($0)/\(sub)" } }
  }
}

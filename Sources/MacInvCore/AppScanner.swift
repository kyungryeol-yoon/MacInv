import Foundation

/// 설치된 앱(.app) 찾기.
/// Go판은 /Applications 바로 아래만 봤다 — 폴더 안 앱(`Chrome Apps.localized/X.app`)과
/// `/Users/Shared/Riot Games/Riot Client.app`처럼 앱 폴더 밖에 깔리는 본체를 놓쳤다(2026-11-21 Riot 사례).
public enum AppScanner {
  public static func roots(home: String) -> [String] {
    ["/Applications", "/Applications/Utilities", "\(home)/Applications", "/Users/Shared"]
  }

  /// 폴더 이름이 번들(앱이 아닌 .bundle·.framework 등)이면 그 안으로 들어가지 않는다.
  static let bundleExtensions: Set<String> = ["app", "bundle", "framework", "plugin", "kext", "appex", "xpc", "pkg", "mpkg"]

  /// 루트 아래 .app 경로들: 루트 바로 아래 + 일반 폴더 한 단계 안. 중복(Utilities가 /Applications 아래이기도 함) 제거.
  /// list: 폴더 → (이름, 폴더인지) 목록. 파일 시스템을 주입받아 테스트한다.
  public static func candidates(roots: [String], list: (String) -> [(name: String, isDir: Bool)]) -> [String] {
    var seen = Set<String>(), out: [String] = []
    func add(_ p: String) { if seen.insert(p).inserted { out.append(p) } }
    for root in roots {
      for e in list(root) where e.isDir && !e.name.hasPrefix(".") {
        let p = (root as NSString).appendingPathComponent(e.name)
        let ext = (e.name as NSString).pathExtension.lowercased()
        if ext == "app" { add(p); continue }
        if bundleExtensions.contains(ext) { continue }
        if roots.contains(p) { continue }   // /Applications/Utilities는 따로 훑는다
        for c in list(p) where c.isDir && (c.name as NSString).pathExtension.lowercased() == "app" {
          add((p as NSString).appendingPathComponent(c.name))
        }
      }
    }
    return out
  }

  public static func scan(home: String = NSHomeDirectory(), casks: [BrewCask] = []) -> [AppItem] {
    let fm = FileManager.default
    let paths = candidates(roots: roots(home: home)) { dir in
      guard let names = try? fm.contentsOfDirectory(atPath: dir) else { return [] }
      return names.map { n in
        var isDir: ObjCBool = false
        fm.fileExists(atPath: (dir as NSString).appendingPathComponent(n), isDirectory: &isDir)
        return (n, isDir.boolValue)
      }
    }
    let caskByApp = caskIndex(casks)
    return paths.map { read($0, caskByApp: caskByApp) }
      .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
  }

  /// "DiSwitch.app" → cask 토큰.
  public static func caskIndex(_ casks: [BrewCask]) -> [String: String] {
    var m: [String: String] = [:]
    for c in casks { for a in c.appNames { m[(a as NSString).lastPathComponent] = c.token } }
    return m
  }

  static func read(_ path: String, caskByApp: [String: String]) -> AppItem {
    let url = URL(fileURLWithPath: path)
    let info = NSDictionary(contentsOf: url.appendingPathComponent("Contents/Info.plist")) as? [String: Any] ?? [:]
    let bundleID = info["CFBundleIdentifier"] as? String
    let version = (info["CFBundleShortVersionString"] as? String) ?? (info["CFBundleVersion"] as? String)
    let added = (try? url.resourceValues(forKeys: [.addedToDirectoryDateKey]))?.addedToDirectoryDate
    let hasReceipt = FileManager.default.fileExists(atPath: url.appendingPathComponent("Contents/_MASReceipt/receipt").path)
    let file = url.lastPathComponent
    return AppItem(name: (file as NSString).deletingPathExtension, path: path, bundleID: bundleID, version: version,
                   dateAdded: added, origin: origin(bundleID: bundleID, hasMASReceipt: hasReceipt, cask: caskByApp[file]))
  }

  /// 출처 판정 — 순수 함수. brew cask가 App Store보다 우선(같은 앱을 cask로 깔았으면 brew가 관리한다).
  public static func origin(bundleID: String?, hasMASReceipt: Bool, cask: String?) -> AppOrigin {
    if let cask { return .brewCask(cask) }
    if hasMASReceipt { return .appStore }
    if let id = bundleID, id.hasPrefix("com.apple.") { return .apple }
    return .other
  }
}

/// `du -sk a b c` 출력 → 경로별 KB(순수 함수). 권한 오류로 빠진 경로는 결과에 없다.
public enum DiskUsage {
  public static func parse(_ out: String) -> [String: Int] {
    var m: [String: Int] = [:]
    for line in out.split(separator: "\n") {
      guard let tab = line.firstIndex(of: "\t"), let kb = Int(line[..<tab].trimmingCharacters(in: .whitespaces)) else { continue }
      m[String(line[line.index(after: tab)...])] = kb
    }
    return m
  }

  /// 여러 경로를 du 한 번으로 잰다.
  public static func measure(_ paths: [String], runner: CommandRunner, timeout: Double = 300) async -> [String: Int] {
    guard !paths.isEmpty, let r = try? await runner.run("du", ["-sk"] + paths, timeout: timeout, okCodes: [0, 1]) else { return [:] }
    return parse(r.stdout)
  }
}

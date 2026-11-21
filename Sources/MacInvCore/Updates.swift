import Foundation

/// 업데이트 대기 한 줄.
public struct OutdatedItem: Sendable, Identifiable, Hashable {
  public enum Source: String, Sendable, Hashable {
    case brewFormula, brewCask, npm, mas
    public var label: String {
      switch self { case .brewFormula: return "brew"; case .brewCask: return "brew cask"; case .npm: return "npm 전역"; case .mas: return "App Store" }
    }
  }
  public var id: String { "\(source.rawValue):\(name)" }
  public let name: String
  public let source: Source
  public let current: String
  public let latest: String
  /// brew pin으로 고정됨 — 업데이트하지 않는다
  public let pinned: Bool
  /// App Store 앱 ID(mas upgrade에 씀)
  public let masID: String?

  /// 런타임이 깨질 수 있는 폭으로 올라감(node 25→26 등). "전부 업데이트"에서 빼고 따로 알린다.
  public var runtimeMajor: Bool { source == .brewFormula && Updates.runtimeMajorJump(name: name, current: current, latest: latest) }
}

public enum Updates {
  /// 런타임별 "메이저" 자릿수. **go/rust는 1마디라 1.x 안에서 경고하지 않는다 — 의도된 결정**
  /// (1.x 하위 호환이 정책. 릴리스마다 경고가 뜨면 진짜 위험한 node 경고까지 무시하게 된다).
  static let runtimeCycleParts: [String: Int] = ["node": 1, "openjdk": 1, "deno": 1, "bun": 1, "go": 1, "rust": 1,
                                                 "python": 2, "ruby": 2, "php": 2]

  /// 순수: 런타임 포뮬러가 사이클을 넘어가는가. `node@22`처럼 @가 붙은 고정 포뮬러는 대상 아님.
  public static func runtimeMajorJump(name: String, current: String, latest: String) -> Bool {
    guard !name.contains("@"), let n = runtimeCycleParts[name.lowercased()] else { return false }
    guard let c = cyclePrefix(current, parts: n), let l = cyclePrefix(latest, parts: n) else { return false }
    return c != l
  }

  /// 버전 앞 n마디("25.9.0_2", 1 → "25"). 숫자가 모자라면 nil.
  public static func cyclePrefix(_ version: String, parts n: Int) -> String? {
    var v = Substring(version)
    while v.first == "v" { v = v.dropFirst() }
    var out: [String] = []
    for part in v.split(separator: ".") {
      let num = part.prefix { $0.isASCII && $0.isNumber }
      if num.isEmpty { break }
      out.append(String(num))
      if out.count == n { break }
    }
    return out.count == n ? out.joined(separator: ".") : nil
  }

  /// 모두 모으기. 도구가 없으면 건너뛰고, 실패는 경고로.
  public static func scan(runner: CommandRunner) async -> (items: [OutdatedItem], warnings: [String]) {
    async let brew = run(runner, "brew", ["outdated", "--json=v2"], timeout: 300) { Parse.brewOutdated(Data($0.utf8)) }
    // 업데이트가 있으면 exit 1이 정상
    async let npm = run(runner, "npm", ["outdated", "-g", "--json"], okCodes: [0, 1], timeout: 180) { Parse.npmOutdated(Data($0.utf8)) }
    async let mas = run(runner, "mas", ["outdated"], timeout: 120) { Parse.masOutdated($0) }
    var items: [OutdatedItem] = [], warnings: [String] = []
    for r in await [brew, npm, mas] { items += r.items; warnings += r.warnings }
    return (items, warnings)
  }

  static func run(_ runner: CommandRunner, _ tool: String, _ args: [String], okCodes: Set<Int32> = [0], timeout: Double,
                  parse: (String) -> [OutdatedItem]) async -> (items: [OutdatedItem], warnings: [String]) {
    do {
      let r = try await runner.run(tool, args, timeout: timeout, okCodes: okCodes)
      return (parse(r.stdout), [])
    } catch CommandError.notFound { return ([], []) }
    catch { return ([], ["\(error)"]) }
  }
}

extension Parse {
  /// `brew outdated --json=v2`
  public static func brewOutdated(_ data: Data) -> [OutdatedItem] {
    guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return [] }
    func items(_ key: String, _ source: OutdatedItem.Source) -> [OutdatedItem] {
      (root[key] as? [[String: Any]] ?? []).compactMap { o in
        guard let name = o["name"] as? String, let latest = o["current_version"] as? String else { return nil }
        let installed = (o["installed_versions"] as? [String])?.last ?? "?"
        return OutdatedItem(name: name, source: source, current: installed, latest: latest,
                            pinned: o["pinned"] as? Bool ?? false, masID: nil)
      }
    }
    return items("formulae", .brewFormula) + items("casks", .brewCask)
  }

  /// `npm outdated -g --json` — 업데이트가 없으면 빈 출력
  public static func npmOutdated(_ data: Data) -> [OutdatedItem] {
    guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return [] }
    return root.compactMap { name, v in
      guard let d = v as? [String: Any], let latest = d["latest"] as? String else { return nil }
      return OutdatedItem(name: name, source: .npm, current: d["current"] as? String ?? "?", latest: latest, pinned: false, masID: nil)
    }.sorted { $0.name < $1.name }
  }

  /// `mas outdated` — `497799835 Xcode (26.4.0 -> 26.4.1)`
  public static func masOutdated(_ out: String) -> [OutdatedItem] {
    let re = try! NSRegularExpression(pattern: #"^(\d+)\s+(.+?)\s+\(([^)]*?)\s*->\s*([^)]*)\)\s*$"#)
    return out.split(separator: "\n").compactMap { raw in
      let line = raw.trimmingCharacters(in: .whitespaces)
      guard let m = re.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)) else { return nil }
      func g(_ i: Int) -> String { Range(m.range(at: i), in: line).map { String(line[$0]).trimmingCharacters(in: .whitespaces) } ?? "" }
      return OutdatedItem(name: g(2), source: .mas, current: g(3), latest: g(4), pinned: false, masID: g(1))
    }
  }
}

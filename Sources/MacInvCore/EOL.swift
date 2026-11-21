import Foundation

/// 런타임 지원 종료(EOL)·최신 버전. 목적은 최신 강요가 아니라 판단 근거.
/// - 출처: endoflife.date `/api/<제품>.json`. java는 `java`/`openjdk` 슬러그가 없어(404) eclipse-temurin.
/// - **조회 실패·오프라인은 에러가 아니다** — 조용히 생략하고 설치 현황만 보여 준다. 7일 캐시(실패하면 오래된 캐시라도 쓴다).
/// - **런타임은 실행 대상이 아니다** — 올리는 명령은 보여 주기만 한다.
public struct EOLCycle: Sendable, Hashable {
  public let cycle: String
  public let latest: String
  /// 날짜 문자열("2029-04-30"), 미정(false → nil + rolling), 이미 끝남(true)
  public let eol: EOLValue
  public enum EOLValue: Sendable, Hashable { case date(String), rolling, ended, unknown }
}

public struct RuntimeEOL: Sendable, Hashable {
  public enum Status: Sendable, Hashable { case passed, soon, supported, rolling, unknown }
  public enum Update: Sendable, Hashable { case current, patch, major, unknown }
  public let cycle: String
  /// 내 사이클 안의 최신 패치
  public let latestVersion: String
  /// 나와 있는 가장 최신 사이클 — 패치와 메이저는 위험도가 전혀 다르므로 따로 둔다
  public let latestCycle: String
  public let eolDate: String?
  public let status: Status
  public let update: Update
}

public enum EOL {
  public static let products = ["python": "python", "node": "nodejs", "go": "go", "java": "eclipse-temurin",
                                "ruby": "ruby", "php": "php", "rust": "rust"]
  static let ttl: TimeInterval = 7 * 24 * 3600

  // MARK: 순수 판정

  public static func parse(_ data: Data) -> [EOLCycle]? {
    guard let arr = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return nil }
    return arr.compactMap { d in
      let cycle = (d["cycle"] as? String) ?? (d["cycle"] as? NSNumber)?.stringValue
      guard let cycle, !cycle.isEmpty else { return nil }
      let eol: EOLCycle.EOLValue
      if let s = d["eol"] as? String { eol = .date(s) }
      else if let b = d["eol"] as? Bool { eol = b ? .ended : .rolling }
      else { eol = .unknown }
      return EOLCycle(cycle: cycle, latest: (d["latest"] as? String) ?? "", eol: eol)
    }
  }

  /// 버전에서 숫자 버전만("Python 3.12.13" → "3.12.13", "v25.9.0_2" → "25.9.0").
  public static func normalizeVersion(_ s: String) -> String {
    Runtimes.extractVersion(s) ?? String(s.drop { !$0.isNumber }.prefix { $0.isNumber })
  }

  /// **가장 긴 접두사 일치**: 사이클 자릿수가 제각각("3.9" python, "26" node, "1.26" go)이라
  /// 짧은 쪽이 먼저 걸리면 오답이 된다("1.2"가 "1.26.5"를 집으면 안 됨).
  public static func matchCycle(_ version: String, _ cycles: [EOLCycle]) -> EOLCycle? {
    let v = normalizeVersion(version)
    guard !v.isEmpty else { return nil }
    return cycles.filter { v == $0.cycle || v.hasPrefix($0.cycle + ".") }.max { $0.cycle.count < $1.cycle.count }
  }

  /// 종료일 → 상태. 1년 안이면 "곧 종료".
  public static func status(_ eol: EOLCycle.EOLValue, now: Date) -> (date: String?, status: RuntimeEOL.Status) {
    switch eol {
    case .rolling: return (nil, .rolling)
    case .ended: return (nil, .passed)
    case .unknown: return (nil, .unknown)
    case .date(let s):
      guard let d = dateFormatter.date(from: s) else { return (nil, .unknown) }
      if d < now { return (s, .passed) }
      if d < Calendar(identifier: .gregorian).date(byAdding: .year, value: 1, to: now)! { return (s, .soon) }
      return (s, .supported)
    }
  }

  static let dateFormatter: DateFormatter = {
    let f = DateFormatter(); f.locale = Locale(identifier: "en_US_POSIX"); f.timeZone = TimeZone(identifier: "UTC"); f.dateFormat = "yyyy-MM-dd"; return f
  }()

  /// 세 갈래: 최신 사이클이 내 것과 다르면 메이저, 같은 사이클에서 뒤처졌으면 패치, 아니면 현재.
  public static func compare(installed: String, latestInCycle: String, newestCycle: String, myCycle: String) -> RuntimeEOL.Update {
    if installed.isEmpty { return .unknown }
    if !newestCycle.isEmpty, newestCycle != myCycle { return .major }
    if latestInCycle.isEmpty { return .unknown }
    return compareSemver(normalizeVersion(installed), normalizeVersion(latestInCycle)) < 0 ? .patch : .current
  }

  public static func compareSemver(_ a: String, _ b: String) -> Int {
    let x = a.split(separator: ".").map { Int($0) ?? 0 }, y = b.split(separator: ".").map { Int($0) ?? 0 }
    for i in 0..<max(x.count, y.count) {
      let p = i < x.count ? x[i] : 0, q = i < y.count ? y[i] : 0
      if p != q { return p - q }
    }
    return 0
  }

  /// "26.13.1+build" → "26.13.1"
  static func normalizeLatest(_ s: String) -> String {
    if let i = s.firstIndex(where: { $0 == "+" || $0 == "-" }), i != s.startIndex { return String(s[..<i]) }
    return s
  }

  public static func evaluate(version: String?, cycles: [EOLCycle], now: Date = Date()) -> RuntimeEOL? {
    guard let version, let c = matchCycle(version, cycles) else { return nil }
    let latest = normalizeLatest(c.latest), newest = cycles.first?.cycle ?? ""
    let (date, status) = status(c.eol, now: now)
    return RuntimeEOL(cycle: c.cycle, latestVersion: latest, latestCycle: newest, eolDate: date, status: status,
                      update: compare(installed: version, latestInCycle: latest, newestCycle: newest, myCycle: c.cycle))
  }

  /// 몇 개월 남았나(최소 0).
  public static func monthsUntil(_ date: String, now: Date = Date()) -> Int? {
    guard let d = dateFormatter.date(from: date) else { return nil }
    return max(0, Int(d.timeIntervalSince(now) / 86400 / 30.44))
  }

  /// 올리는 명령(보여 주기만). 관리 주체마다 다르다. 시스템 런타임은 제자리 업그레이드가 안 되니 "따로 설치".
  public static func suggestCommand(runtime: String, manager: Manager, latestCycle: String, hasMise: Bool) -> String? {
    let cycle = latestCycle.isEmpty ? "latest" : latestCycle
    let formula = runtime == "java" ? "openjdk" : runtime
    switch manager {
    case .brew: return "brew upgrade \(formula)"
    case .nvm: return "nvm install --lts && nvm alias default lts/*"
    case .pyenv: return "pyenv install \(cycle) && pyenv global \(cycle)"
    case .rustup: return "rustup update"
    case .mise: return "mise use -g \(runtime)@\(cycle)"
    case .asdf: return "asdf install \(runtime) latest && asdf global \(runtime) latest"
    case .sdkman: return "sdk upgrade \(runtime)"
    case .system:
      return hasMise ? "mise use -g \(runtime)@\(cycle)   (시스템 버전은 그대로 두고 앞에 둡니다)"
                     : "brew install \(formula)   (시스템 버전은 업그레이드할 수 없어 따로 설치합니다)"
    default: return nil
    }
  }

  // MARK: 조회 + 캐시

  struct CacheEntry: Codable { let fetchedAt: Date; let json: Data }

  /// 앱 캐시 폴더(번들 ID 폴더 — SelfIdentity가 자기 데이터로 판정해 정리 대상에서 빠진다).
  public static func cacheURL() -> URL {
    let caches = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
    return caches.appendingPathComponent(SelfIdentity.defaultBundleID).appendingPathComponent("eol-cache.json")
  }

  /// 런타임 이름 → 사이클 목록. 캐시가 7일 안이면 캐시, 아니면 조회(실패하면 오래된 캐시라도).
  public static func cycles(for names: [String], cacheURL: URL = cacheURL(), now: Date = Date(),
                            fetch: @Sendable (String) async -> Data? = fetchProduct) async -> [String: [EOLCycle]] {
    var cache = (try? JSONDecoder().decode([String: CacheEntry].self, from: Data(contentsOf: cacheURL))) ?? [:]
    var out: [String: [EOLCycle]] = [:], changed = false
    for name in names {
      guard let product = products[name] else { continue }
      if let e = cache[product], now.timeIntervalSince(e.fetchedAt) < ttl, let c = parse(e.json) { out[name] = c; continue }
      if let data = await fetch(product), let c = parse(data), !c.isEmpty {
        out[name] = c; cache[product] = CacheEntry(fetchedAt: now, json: data); changed = true
      } else if let e = cache[product], let c = parse(e.json) {
        out[name] = c   // 오프라인: 오래된 캐시라도
      }
    }
    if changed {
      try? FileManager.default.createDirectory(at: cacheURL.deletingLastPathComponent(), withIntermediateDirectories: true)
      try? JSONEncoder().encode(cache).write(to: cacheURL)
    }
    return out
  }

  @Sendable public static func fetchProduct(_ product: String) async -> Data? {
    guard let url = URL(string: "https://endoflife.date/api/\(product).json") else { return nil }
    var req = URLRequest(url: url, timeoutInterval: 6)
    req.setValue("MacInv", forHTTPHeaderField: "User-Agent")
    guard let (data, resp) = try? await URLSession.shared.data(for: req), (resp as? HTTPURLResponse)?.statusCode == 200 else { return nil }
    return data
  }
}

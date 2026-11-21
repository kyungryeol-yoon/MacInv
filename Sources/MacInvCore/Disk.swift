import Foundation

/// "왜 디스크가 꽉 찼나" — 알려진 캐시·모델·빌드 산출물 위치를 재고, 인벤토리와 대조해 고아 캐시를 찾는다.
/// (원래 동기: 시스템 설정 "문서" 57 GB인데 Finder로는 11 GB — ~/.ollama 모델과 개발 캐시였다)
public enum Tier: String, Sendable, Codable, Hashable {
  case safe, artifact, expensive, never
  public var label: String {
    switch self { case .safe: return "안전(재생성)"; case .artifact: return "빌드 산출물"; case .expensive: return "재다운로드 비쌈"; case .never: return "정리 안 함" }
  }
  /// 유일한 판정: 기본은 safe+artifact, expensive는 옵트인, **never는 어떤 옵션으로도 안 뚫린다**.
  public func cleanable(includeExpensive: Bool) -> Bool {
    switch self { case .safe, .artifact: return true; case .expensive: return includeExpensive; case .never: return false }
  }
}

public struct DiskLocation: Sendable, Decodable, Hashable {
  public let id: String
  public let name: String
  public let globs: [String]
  public let tool: String
  public let owner_apps: [String]?
  public let tier: Tier
  public let cleanup: [[String]]?
  public let trash: Bool?
  public let group_by: String?
  public let note: String?
}

public struct DiskEntry: Sendable, Identifiable, Hashable {
  public var id: String { location.id }
  public let location: DiskLocation
  public let paths: [String]
  /// 경로마다 제 용량 — 합계를 경로마다 넣으면 경로 N개일 때 회수 예상이 N배가 된다(Go판 실측)
  public var pathSizes: [String: Int] = [:]
  public var sizeKB: Int { pathSizes.values.reduce(0, +) }
  /// 캐시는 있는데 쓰는 도구·앱이 하나도 안 남음
  public var orphan = false
  /// 와일드카드 위치(Electron 캐시)의 앱별 내역 — "66 GB" 한 줄로는 어느 앱을 손볼지 모른다
  public var breakdown: [(name: String, kb: Int)] = []
  public var partial = false
  public var deniedPaths: [String] = []
  /// 위치가 다른 곳으로 가는 바로가기(심볼릭 링크) → 실제 위치. du는 링크를 따라가지 않아 0으로 잡힌다 —
  /// 내장 디스크 공간을 안 쓴다는 뜻이라 값은 맞지만, 그 사실을 보여 줘야 한다(2026-11-21 ~/.ollama/models → 외장 SSD 실측)
  public var linkedTo: [String: String] = [:]
  /// 실행 중인 앱의 캐시 경로 → 앱(폴더) 이름 — 정리에서 뺀다
  public var blockedByRunning: [String: String] = [:]
  public var blockedKB: Int { blockedByRunning.keys.reduce(0) { $0 + (pathSizes[$1] ?? 0) } }
  public static func == (a: DiskEntry, b: DiskEntry) -> Bool { a.id == b.id && a.pathSizes == b.pathSizes && a.orphan == b.orphan }
  public func hash(into h: inout Hasher) { h.combine(id) }
}

public struct DiskReport: Sendable {
  public var entries: [DiskEntry] = []
  public var totalKB: Int { entries.reduce(0) { $0 + $1.sizeKB } }
  /// 기본 정리 대상(safe + artifact) — 실행 중이라 빼는 몫은 제외
  public var reclaimableKB: Int { entries.filter { $0.location.tier.cleanable(includeExpensive: false) }.reduce(0) { $0 + $1.sizeKB - $1.blockedKB } }
  /// 실행 중이라 정리에서 뺀 앱별 용량(큰 순) — 앱을 끄고 다시 돌릴지 판단하려면 얼마가 걸려 있는지 알아야 한다
  public var blockedByApp: [(name: String, kb: Int)] {
    var by: [String: Int] = [:]
    for e in entries { for (p, app) in e.blockedByRunning { by[app, default: 0] += e.pathSizes[p] ?? 0 } }
    return by.map { ($0.key, $0.value) }.sorted { $0.1 > $1.1 }
  }
  /// 실행 중 앱 조회 실패 — 보호가 꺼진 상태이므로 반드시 알린다
  public var runningCheckFailed = false
  /// APFS 로컬 스냅샷 이름(nil = 확인 못 함)
  public var snapshots: [String]?
  /// Ollama 모델(nil = ollama 없음·서버 꺼짐)
  public var ollamaModels: [OllamaModel]?
  public var expensiveKB: Int { entries.filter { $0.location.tier == .expensive }.reduce(0) { $0 + $1.sizeKB } }
  public var freeBytes: Int64?
  public var totalBytes: Int64?
  public var scannedAt = Date()
  public init() {}
}

public enum Disk {
  public static let groupByAppSupportDir = "app-support-dir"

  public static let locations: [DiskLocation] = {
    struct T: Decodable { let locations: [DiskLocation] }
    return (try? JSONDecoder().decode(T.self, from: Data(LocationsData.json.utf8)).locations) ?? []
  }()

  // MARK: 경로 펼치기(순수 — fs 주입)

  /// `~/Library/Application Support/*/Code Cache`처럼 **데이터에 적힌** 패턴만 펼친다(한 마디씩 fnmatch).
  /// 앱 이름 같은 바깥 값은 패턴에 끼워 넣지 않으므로 메타문자 문제가 없다.
  public static func expand(_ pattern: String, home: String, fs: FileSystem = .real) -> [String] {
    var p = pattern
    if p == "~" { p = home } else if p.hasPrefix("~/") { p = home + p.dropFirst(1) }
    var current = [""]
    for comp in p.split(separator: "/").map(String.init) {
      var next: [String] = []
      for base in current {
        if comp.contains("*") || comp.contains("?") {
          for name in fs.list(base.isEmpty ? "/" : base) where fnmatch(comp, name, 0) == 0 { next.append(base + "/" + name) }
        } else {
          next.append(base + "/" + comp)
        }
      }
      current = next
    }
    return current.filter { !$0.isEmpty && fs.exists($0) }.sorted()
  }

  /// 경로 마디에 `com.apple.`이 있으면 애플 데이터 — 접두사 블랙리스트(/System 등)는 ~/Library 아래 애플 데이터를 못 거른다.
  public static func hasAppleComponent(_ path: String) -> Bool {
    path.split(separator: "/").contains { $0.hasPrefix("com.apple.") }
  }

  /// 위치 ID → 실제 경로. 보호 경로·애플 데이터 제외, 위치끼리 겹치면 넓은 쪽 하나만(같은 데이터를 두 번 세지 않게).
  public static func resolve(home: String, locations: [DiskLocation] = locations, fs: FileSystem = .real) -> [String: [String]] {
    var all: [PathMatch] = [], owner: [String: String] = [:]
    for l in locations {
      for g in l.globs {
        for m in expand(g, home: home, fs: fs) where !Paths.isProtected(m) && !hasAppleComponent(m) && owner[m] == nil {
          owner[m] = l.id
          all.append(PathMatch(path: m, confidence: .high, reason: l.id))
        }
      }
    }
    var out: [String: [String]] = [:]
    for m in Paths.dedupContained(all) { out[owner[m.path]!, default: []].append(m.path) }
    return out
  }

  /// `~/Library/Application Support/<앱>/…` → "<앱>" (Partitions 아래도 첫 마디라 앱 단위로 합쳐진다).
  static func appSupportDirName(home: String, path: String) -> String? {
    let base = "\(home)/Library/Application Support/"
    guard path.hasPrefix(base) else { return nil }
    return path.dropFirst(base.count).split(separator: "/").first.map(String.init)
  }

  /// 앱별 합계(큰 순, 같으면 이름순).
  public static func groupUsage(home: String, groupBy: String?, sizes: [String: Int]) -> [(name: String, kb: Int)] {
    guard groupBy == groupByAppSupportDir else { return [] }
    var by: [String: Int] = [:]
    for (p, kb) in sizes { if let n = appSupportDirName(home: home, path: p) { by[n, default: 0] += kb } }
    return by.map { ($0.key, $0.value) }.sorted { $0.1 != $1.1 ? $0.1 > $1.1 : $0.0 < $1.0 }
  }

  // MARK: 고아 캐시(오탐 방지 3중 장치 — 하나라도 빼면 멀쩡한 캐시가 고아로 뜬다)

  /// 넣을 때와 찾을 때 **같은 함수**로 정규화("Android Studio" → android studio / androidstudio). 한쪽만 바꾸면 매칭이 조용히 깨진다.
  static func toolKeys(_ name: String) -> [String] {
    let lower = name.trimmingCharacters(in: .whitespaces).lowercased()
    guard !lower.isEmpty else { return [] }
    let packed = lower.replacingOccurrences(of: " ", with: "").replacingOccurrences(of: "-", with: "").replacingOccurrences(of: "_", with: "")
    return packed == lower ? [lower] : [lower, packed]
  }

  /// 별칭(도구 ← 그걸 제공하는 것). 오탐 방지용이라 **완화 금지**.
  static let toolAliases = ["npm": "node", "npx": "node", "pip": "python", "pip3": "python", "xcodebuild": "xcode", "simctl": "xcode", "mvn": "java"]
  /// 이름이 다른 실행 파일(PATH 2차 확인용).
  static let toolBinaries = ["pip": ["pip", "pip3"], "python": ["python3", "python"], "simctl": ["xcrun"],
                             "orbstack": ["orb", "orbctl"], "huggingface-cli": ["huggingface-cli", "hf"]]

  /// ① 인벤토리: brew·npm·pipx·cargo·go·앱·런타임 이름 + 앱 Bundle ID 마지막 마디.
  public static func installedTools(_ inv: Inventory) -> Set<String> {
    var set = Set<String>()
    func add(_ s: String) { toolKeys(s).forEach { set.insert($0) } }
    if !inv.formulae.isEmpty || !inv.casks.isEmpty { add("brew") }
    inv.formulae.forEach { add($0.name) }
    inv.casks.forEach { add($0.token) }
    for p in inv.packages {
      add(p.name)
      switch p.manager { case .npm: add("npm"); add("node"); case .cargo: add("cargo"); case .go: add("go"); default: break }
    }
    for a in inv.apps {
      add(a.name)
      if let id = a.bundleID, let last = id.split(separator: ".").last { add(String(last)) }
    }
    inv.runtimes.filter(\.installed).forEach { add($0.name) }
    for (tool, provider) in toolAliases where set.contains(provider) { set.insert(tool) }
    return set
  }

  /// ② PATH 2차 확인: 인벤토리가 낡아 런타임이 비면 python이 없다고 보고 pip 캐시를 오탐한다(Go판 실측).
  public static func detectTools(_ inv: Inventory, isOnPath: (String) -> Bool) -> Set<String> {
    var set = installedTools(inv)
    for l in locations where !l.tool.isEmpty && !anyInstalled(set, l.tool) {
      if (toolBinaries[l.tool] ?? [l.tool]).contains(where: isOnPath) { toolKeys(l.tool).forEach { set.insert($0) } }
    }
    return set
  }

  static func anyInstalled(_ set: Set<String>, _ name: String) -> Bool { toolKeys(name).contains(where: set.contains) }

  /// ③ owner_apps까지 보고 판정(순수). 도구를 모르거나 never면 판정하지 않는다.
  public static func isOrphan(_ l: DiskLocation, installed: Set<String>) -> Bool {
    if l.tool.isEmpty || l.tier == .never { return false }
    if anyInstalled(installed, l.tool) { return false }
    if (l.owner_apps ?? []).contains(where: { anyInstalled(installed, $0) }) { return false }
    return true
  }

  // MARK: 측정

  /// 경로 중 바로가기(심볼릭 링크)인 것 → 실제 위치(절대 경로).
  static func symlinkTargets(_ paths: [String]) -> [String: String] {
    var out: [String: String] = [:]
    for p in paths {
      guard let t = try? FileManager.default.destinationOfSymbolicLink(atPath: p) else { continue }
      out[p] = t.hasPrefix("/") ? t : ((p as NSString).deletingLastPathComponent as NSString).appendingPathComponent(t)
    }
    return out
  }

  /// `/Volumes/<디스크>/…` → 디스크 이름(외장·다른 볼륨). 아니면 nil(순수).
  public static func externalVolume(_ path: String) -> String? {
    let parts = path.split(separator: "/")
    return parts.count >= 2 && parts[0] == "Volumes" ? String(parts[1]) : nil
  }

  /// 실행 중인 앱의 캐시 경로(순수). 앱 폴더 단위(Application Support/<앱>)로 묶이는 위치만 대상.
  public static func blocked(home: String, groupBy: String?, paths: [String], running: [String]) -> [String: String] {
    guard groupBy == groupByAppSupportDir else { return [:] }
    var out: [String: String] = [:]
    for p in paths { if let app = appSupportDirName(home: home, path: p), Running.matches(dirName: app, running: running) { out[p] = app } }
    return out
  }

  public static func scan(runner: CommandRunner, inventory: Inventory?, running: [String]?, home: String = NSHomeDirectory()) async -> DiskReport {
    let byID = resolve(home: home)
    let tools = inventory.map { inv in detectTools(inv) { runner.resolve($0) != nil } }
    var entries: [DiskEntry] = locations.compactMap { l in
      guard let ps = byID[l.id], !ps.isEmpty else { return nil }   // 이 맥에 없는 위치는 조용히 건너뜀
      var e = DiskEntry(location: l, paths: ps)
      if let tools { e.orphan = isOrphan(l, installed: tools) }
      return e
    }
    await withTaskGroup(of: (Int, [String: Int], Bool, [String]).self) { group in
      var next = 0
      func launch(_ i: Int) {
        let ps = entries[i].paths
        group.addTask { let m = await DiskUsage.measureDetailed(ps, runner: runner, timeout: 120); return (i, m.sizes, m.partial, m.denied) }
      }
      while next < min(6, entries.count) { launch(next); next += 1 }
      for await (i, sizes, partial, denied) in group {
        entries[i].pathSizes = sizes; entries[i].partial = partial; entries[i].deniedPaths = denied
        entries[i].breakdown = groupUsage(home: home, groupBy: entries[i].location.group_by, sizes: sizes)
        entries[i].linkedTo = symlinkTargets(entries[i].paths)
        entries[i].blockedByRunning = blocked(home: home, groupBy: entries[i].location.group_by, paths: entries[i].paths, running: running ?? [])
        if next < entries.count { launch(next); next += 1 }
      }
    }
    var r = DiskReport()
    r.entries = entries.sorted { $0.sizeKB > $1.sizeKB }
    r.runningCheckFailed = running == nil
    async let snaps = Snapshots.scan(runner: runner)
    async let models = Ollama.scan(runner: runner)
    r.snapshots = await snaps
    r.ollamaModels = await models
    if let v = try? URL(fileURLWithPath: "/").resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey, .volumeTotalCapacityKey]) {
      r.freeBytes = v.volumeAvailableCapacityForImportantUsage; r.totalBytes = v.volumeTotalCapacity.map(Int64.init)
    }
    return r
  }
}

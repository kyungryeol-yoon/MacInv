import Foundation

/// 패키지 관리자 출력 파서(순수 함수 — 실제 출력 샘플로 테스트) + 수집.
public enum Parse {
  /// `brew info --json=v2 --installed`
  /// cask의 앱 이름은 artifacts의 `{"app": ["X.app"]}`(또는 `{"app": [{"target": "X.app"}]}`)에 있다.
  /// Go판은 문자열 항목만 봐서 실제 출력에선 늘 비어 있었다(2026-11-21 발견).
  public static func brewInfo(_ data: Data) -> (formulae: [BrewFormula], casks: [BrewCask])? {
    guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return nil }
    let formulae: [BrewFormula] = (root["formulae"] as? [[String: Any]] ?? []).compactMap { f in
      guard let name = f["name"] as? String else { return nil }
      let inst = (f["installed"] as? [[String: Any]])?.first
      return BrewFormula(name: name, version: inst?["version"] as? String,
                         onRequest: inst?["installed_on_request"] as? Bool ?? false,
                         dependencies: f["dependencies"] as? [String] ?? [])
    }
    let casks: [BrewCask] = (root["casks"] as? [[String: Any]] ?? []).compactMap { c in
      guard let token = c["token"] as? String else { return nil }
      return BrewCask(token: token, version: c["installed"] as? String, appNames: caskApps(c["artifacts"]))
    }
    return (formulae.sorted { $0.name < $1.name }, casks.sorted { $0.token < $1.token })
  }

  static func caskApps(_ artifacts: Any?) -> [String] {
    var out: [String] = []
    func take(_ v: Any) {
      if let s = v as? String, s.lowercased().hasSuffix(".app") { out.append(s) }
      else if let d = v as? [String: Any], let t = d["target"] as? String, t.lowercased().hasSuffix(".app") { out.append(t) }
    }
    for a in artifacts as? [Any] ?? [] {
      if let d = a as? [String: Any], let apps = d["app"] as? [Any] { apps.forEach(take) }
      else if let arr = a as? [Any] { arr.forEach(take) }   // 옛 형식
      else { take(a) }
    }
    return out
  }

  /// `npm ls -g --depth=0 --json`
  public static func npmLs(_ data: Data) -> [PackageItem] {
    guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
          let deps = root["dependencies"] as? [String: Any] else { return [] }
    return deps.map { name, v in
      PackageItem(name: name, version: (v as? [String: Any])?["version"] as? String, manager: .npm, path: nil)
    }.sorted { $0.name < $1.name }
  }

  /// `pipx list --json`
  public static func pipxList(_ data: Data) -> [PackageItem] {
    guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
          let venvs = root["venvs"] as? [String: Any] else { return [] }
    return venvs.map { name, v in
      let main = ((v as? [String: Any])?["metadata"] as? [String: Any])?["main_package"] as? [String: Any]
      return PackageItem(name: name, version: main?["package_version"] as? String, manager: .pipx, path: nil)
    }.sorted { $0.name < $1.name }
  }

  /// `cargo install --list` — 들여쓰기 없는 줄이 패키지(`ripgrep v14.1.0:`), 들여쓴 줄은 실행 파일 이름.
  public static func cargoList(_ out: String) -> [PackageItem] {
    out.split(separator: "\n", omittingEmptySubsequences: true).compactMap { raw in
      let line = String(raw)
      guard let first = line.first, first != " ", first != "\t" else { return nil }
      var s = line.trimmingCharacters(in: .whitespaces)
      if s.hasSuffix(":") { s.removeLast() }
      let parts = s.split(separator: " ", maxSplits: 2).map(String.init)
      guard let name = parts.first else { return nil }
      var ver = parts.count > 1 ? parts[1] : nil
      if let v = ver, v.hasPrefix("v") { ver = String(v.dropFirst()) }
      return PackageItem(name: name, version: ver, manager: .cargo, path: nil)
    }
  }

  /// `mas list` — `497799835  Xcode  (26.4.1)`
  public static func masList(_ out: String) -> [PackageItem] {
    let re = try! NSRegularExpression(pattern: #"^(\d+)\s+(.+?)\s+\(([^)]*)\)\s*$"#)
    return out.split(separator: "\n").compactMap { raw in
      let line = raw.trimmingCharacters(in: .whitespaces)
      guard let m = re.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)),
            let n = Range(m.range(at: 2), in: line), let v = Range(m.range(at: 3), in: line), let id = Range(m.range(at: 1), in: line)
      else { return nil }
      return PackageItem(name: String(line[n]), version: String(line[v]), manager: .mas, path: String(line[id]))
    }
  }
}

/// 전체 인벤토리 수집. 도구가 없으면 그 소스는 조용히 건너뛴다. 소스끼리는 병렬.
public enum InventoryScanner {
  public static func scan(runner: CommandRunner, home: String = NSHomeDirectory()) async -> Inventory {
    async let brew = scanBrew(runner)
    async let npm = scanSimple(runner, "npm", ["ls", "-g", "--depth=0", "--json"], okCodes: [0, 1]) { Parse.npmLs(Data($0.utf8)) }
    async let pipx = scanSimple(runner, "pipx", ["list", "--json"]) { Parse.pipxList(Data($0.utf8)) }
    async let cargo = scanSimple(runner, "cargo", ["install", "--list"]) { Parse.cargoList($0) }
    async let mas = scanSimple(runner, "mas", ["list"]) { Parse.masList($0) }
    async let runtimes = Runtimes.scan(runner: runner, home: home)

    var inv = Inventory()
    let b = await brew
    inv.formulae = b.formulae; inv.casks = b.casks
    inv.apps = AppScanner.scan(home: home, casks: b.casks)
    var warnings = b.warnings
    for r in await [npm, pipx, cargo, mas] { inv.packages += r.items; warnings += r.warnings }
    inv.packages += goBin(home: home)
    inv.runtimes = await runtimes
    inv.versionManagers = Runtimes.versionManagers(home: home)
    inv.background = Background.scan(apps: inv.apps, home: home)
    inv.warnings = warnings
    inv.scannedAt = Date()
    return inv
  }

  static func scanBrew(_ runner: CommandRunner) async -> (formulae: [BrewFormula], casks: [BrewCask], warnings: [String]) {
    do {
      let r = try await runner.run("brew", ["info", "--json=v2", "--installed"], timeout: 300)
      guard let p = Parse.brewInfo(Data(r.stdout.utf8)) else { return ([], [], ["brew info: 결과를 읽지 못함"]) }
      return (p.formulae, p.casks, [])
    } catch CommandError.notFound { return ([], [], []) }
    catch { return ([], [], ["\(error)"]) }
  }

  static func scanSimple(_ runner: CommandRunner, _ tool: String, _ args: [String], okCodes: Set<Int32> = [0],
                         parse: (String) -> [PackageItem]) async -> (items: [PackageItem], warnings: [String]) {
    do {
      let r = try await runner.run(tool, args, timeout: 120, okCodes: okCodes)
      return (parse(r.stdout), [])
    } catch CommandError.notFound { return ([], []) }
    catch { return ([], ["\(error)"]) }
  }

  /// ~/go/bin의 실행 파일(외부 명령 없음).
  static func goBin(home: String) -> [PackageItem] {
    let dir = "\(home)/go/bin", fm = FileManager.default
    guard let names = try? fm.contentsOfDirectory(atPath: dir) else { return [] }
    return names.sorted().compactMap { n in
      let p = "\(dir)/\(n)"
      var isDir: ObjCBool = false
      guard !n.hasPrefix("."), fm.fileExists(atPath: p, isDirectory: &isDir), !isDir.boolValue, fm.isExecutableFile(atPath: p) else { return nil }
      return PackageItem(name: n, version: nil, manager: .go, path: p)
    }
  }

  /// 앱 번들 용량을 du 한 번으로 재서 채운다(목록을 먼저 보여 주고 나중에 채움).
  public static func measureBundles(_ apps: [AppItem], runner: CommandRunner) async -> [AppItem] {
    let sizes = await DiskUsage.measure(apps.map(\.path), runner: runner)
    return apps.map { var a = $0; a.bundleKB = sizes[a.path]; return a }
  }
}

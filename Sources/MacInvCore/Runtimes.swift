import Foundation

/// 언어 런타임: 깔려 있나 · 몇 버전인가 · 누가 관리하나(nvm/pyenv/mise/brew…) · 여러 개가 겹치나.
/// 목적은 최신 강요가 아니라 판단 근거(몇 달 만에 돌아왔을 때 한 번에 답하기). EOL·최신 버전은 3단계.
public struct RuntimeOrigin: Sendable, Hashable {
  /// PATH에서 찾은 경로
  public let path: String
  /// 심볼릭 링크를 따라간 실제 경로
  public let resolved: String
  public let manager: Manager
  /// brew 포뮬러 이름·npm 패키지 이름 등(알 수 있을 때)
  public let package: String?
}

public enum Manager: String, Sendable, Hashable {
  case nvm, pyenv, asdf, rustup, sdkman, mise, volta, brew, npm, pipx, cargo, go, system, manual
  public var label: String {
    switch self {
    case .system: return "시스템"
    case .manual: return "직접 설치"
    case .go: return "Go"
    default: return rawValue
    }
  }
}

public struct Runtime: Sendable, Identifiable, Hashable {
  public var id: String { name }
  public let name: String
  public let installed: Bool
  public let version: String?
  /// PATH 순서대로. 첫 번째가 지금 쓰이는 것
  public let origins: [RuntimeOrigin]
  /// 실제 경로가 서로 다른 것이 2개 이상 — 어느 게 쓰이는지 헷갈릴 수 있음
  public var conflict: Bool { Set(origins.map(\.resolved)).count > 1 }
  public var active: RuntimeOrigin? { origins.first }
}

public struct VersionManagerInfo: Sendable, Hashable, Identifiable {
  public var id: String { name }
  public let name: String
  public let versions: [String]
}

public enum Runtimes {
  struct Spec: Sendable { let name: String; let bin: String; let args: [String]; let pattern: String? }

  static let specs: [Spec] = [
    Spec(name: "node", bin: "node", args: ["--version"], pattern: nil),
    Spec(name: "python", bin: "python3", args: ["--version"], pattern: nil),
    Spec(name: "go", bin: "go", args: ["version"], pattern: nil),
    Spec(name: "rust", bin: "rustc", args: ["--version"], pattern: nil),
    Spec(name: "java", bin: "java", args: ["-version"], pattern: nil),
    Spec(name: "ruby", bin: "ruby", args: ["--version"], pattern: nil),
    Spec(name: "php", bin: "php", args: ["--version"], pattern: nil),
    Spec(name: "dotnet", bin: "dotnet", args: ["--version"], pattern: nil),
    // swift --version 첫 줄은 swift-driver 버전이라 그걸 집으면 안 된다
    Spec(name: "swift", bin: "swift", args: ["--version"], pattern: #"Apple Swift version (\d+\.\d+(?:\.\d+)?)"#),
  ]

  /// 버전 명령 출력에서 버전을 뽑는다(순수). pattern이 있으면 첫 캡처 그룹.
  public static func extractVersion(_ out: String, pattern: String? = nil) -> String? {
    func first(_ p: String, group: Int) -> String? {
      guard let re = try? NSRegularExpression(pattern: p),
            let m = re.firstMatch(in: out, range: NSRange(out.startIndex..., in: out)),
            let r = Range(m.range(at: group), in: out) else { return nil }
      return String(out[r])
    }
    if let pattern, let v = first(pattern, group: 1) { return v }
    return first(#"\d+\.\d+(?:\.\d+)?"#, group: 0)
  }

  /// 실제 경로만 보고 관리 주체를 판정(순수). 버전 매니저 경로가 우선.
  public static func classify(_ resolved: String, home: String) -> (Manager, String?) {
    let segs = resolved.split(separator: "/").map(String.init)
    func has(_ d: String) -> Bool { segs.contains(d) }
    if has(".nvm") { return (.nvm, nil) }
    if has(".pyenv") { return (.pyenv, nil) }
    if has(".asdf") { return (.asdf, nil) }
    if has(".rustup") { return (.rustup, nil) }
    if has(".sdkman") { return (.sdkman, nil) }
    if has("mise") { return (.mise, nil) }
    if has(".volta") { return (.volta, nil) }
    if let i = segs.firstIndex(where: { $0 == "Cellar" || $0 == "Caskroom" }), i + 1 < segs.count { return (.brew, segs[i + 1]) }
    if let i = segs.firstIndex(of: "node_modules"), i + 1 < segs.count {
      var p = segs[i + 1]
      if p.hasPrefix("@"), i + 2 < segs.count { p += "/" + segs[i + 2] }
      return (.npm, p)
    }
    if has("pipx") {
      if let j = segs.firstIndex(of: "venvs"), j + 1 < segs.count { return (.pipx, segs[j + 1]) }
      return (.pipx, nil)
    }
    if has(".cargo") { return (.cargo, nil) }
    if resolved.hasPrefix("\(home)/go/bin/") { return (.go, nil) }
    for p in ["/usr/bin/", "/bin/", "/usr/sbin/", "/sbin/", "/usr/libexec/", "/System/", "/Library/Developer/CommandLineTools/", "/Applications/Xcode.app/"]
    where resolved.hasPrefix(p) { return (.system, nil) }
    return (.manual, nil)
  }

  /// PATH의 모든 dir에서 name을 찾는다(which -a). 같은 파일은 한 번만.
  static func findAll(_ name: String, in dirs: [String]) -> [String] {
    var seen = Set<String>()
    return dirs.compactMap { d in
      let p = (d as NSString).appendingPathComponent(name)
      guard FileManager.default.isExecutableFile(atPath: p), seen.insert(p).inserted else { return nil }
      return p
    }
  }

  /// 설치됨 판정(순수): 버전이 안 나오고 시스템 경로뿐이면 macOS가 넣어 둔 껍데기(`/usr/bin/java` → "Unable to locate a Java Runtime").
  public static func isInstalled(origins: [RuntimeOrigin], version: String?) -> Bool {
    if origins.isEmpty { return false }
    if version == nil, origins.allSatisfy({ $0.manager == .system }) { return false }
    return true
  }

  public static func scan(runner: CommandRunner, home: String = NSHomeDirectory()) async -> [Runtime] {
    await withTaskGroup(of: (Int, Runtime).self) { group in
      for (i, spec) in specs.enumerated() {
        group.addTask {
          let origins = findAll(spec.bin, in: runner.searchPath).map { p -> RuntimeOrigin in
            let r = URL(fileURLWithPath: p).resolvingSymlinksInPath().path
            let (m, pkg) = classify(r, home: home)
            return RuntimeOrigin(path: p, resolved: r, manager: m, package: pkg)
          }
          var version: String?
          if let first = origins.first,
             let r = try? await runner.run(first.path, spec.args, timeout: 15, okCodes: Set(0...255)) {
            version = extractVersion(r.stdout + "\n" + r.stderr, pattern: spec.pattern)   // java·swift는 stderr에 찍는다
          }
          let installed = isInstalled(origins: origins, version: version)
          return (i, Runtime(name: spec.name, installed: installed, version: installed ? version : nil, origins: installed ? origins : []))
        }
      }
      var out = [Runtime?](repeating: nil, count: specs.count)
      for await (i, rt) in group { out[i] = rt }
      return out.compactMap { $0 }
    }
  }

  /// 버전 매니저가 관리 중인 버전(외부 명령 없이 폴더만 본다).
  public static func versionManagers(home: String = NSHomeDirectory()) -> [VersionManagerInfo] {
    let fm = FileManager.default
    func list(_ p: String) -> [String] {
      ((try? fm.contentsOfDirectory(atPath: p)) ?? []).filter { !$0.hasPrefix(".") && $0 != "current" }.sorted()
    }
    let single: [(String, String)] = [("nvm", ".nvm/versions/node"), ("pyenv", ".pyenv/versions"), ("rustup", ".rustup/toolchains")]
    let double: [(String, String)] = [("asdf", ".asdf/installs"), ("mise", ".local/share/mise/installs"), ("sdkman", ".sdkman/candidates")]
    var out: [VersionManagerInfo] = []
    for (n, rel) in single {
      let v = list("\(home)/\(rel)")
      if !v.isEmpty { out.append(VersionManagerInfo(name: n, versions: v)) }
    }
    for (n, rel) in double {
      let v = list("\(home)/\(rel)").flatMap { tool in list("\(home)/\(rel)/\(tool)").map { "\(tool) \($0)" } }
      if !v.isEmpty { out.append(VersionManagerInfo(name: n, versions: v)) }
    }
    return out
  }
}

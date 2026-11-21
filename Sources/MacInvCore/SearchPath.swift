import Foundation

/// 메뉴 막대 앱의 PATH.
/// GUI 앱은 셸 환경을 물려받지 않아 PATH가 `/usr/bin:/bin:/usr/sbin:/sbin`뿐이다 — brew·nvm의 node·cargo를 못 찾는다.
/// 그래서 ① 사용자의 로그인 셸에서 실제 PATH를 받아 오고(우선순위 그대로) ② 알려진 위치를 뒤에 덧붙인다(셸 조회가 실패해도 동작).
public enum SearchPath {
  public static let beginMarker = "__MACINV_PATH_BEGIN__"
  public static let endMarker = "__MACINV_PATH_END__"

  /// 셸 조회가 실패해도 찾아야 하는 위치(순서 = 우선순위).
  public static func knownDirs(home: String) -> [String] {
    ["/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin",
     "\(home)/.local/share/mise/shims", "\(home)/.asdf/shims", "\(home)/.volta/bin",
     "\(home)/.cargo/bin", "\(home)/go/bin", "\(home)/.local/bin", "\(home)/.bun/bin",
     "/usr/bin", "/bin", "/usr/sbin", "/sbin"]
  }

  /// 여러 목록을 순서대로 합치고 중복·빈 항목·끝 슬래시를 정리한다.
  public static func merge(_ lists: [String]...) -> [String] {
    var seen = Set<String>(), out: [String] = []
    for list in lists {
      for raw in list {
        var d = raw.trimmingCharacters(in: .whitespaces)
        while d.count > 1, d.hasSuffix("/") { d.removeLast() }
        if d.isEmpty || seen.contains(d) { continue }
        seen.insert(d); out.append(d)
      }
    }
    return out
  }

  /// 셸 출력에서 표식 사이의 PATH만 뽑는다(.zshrc가 무엇을 찍어도 섞이지 않게). 없으면 nil.
  public static func parseMarked(_ output: String) -> [String]? {
    guard let b = output.range(of: beginMarker),
          let e = output.range(of: endMarker, range: b.upperBound..<output.endIndex) else { return nil }
    let dirs = output[b.upperBound..<e.lowerBound].split(separator: ":").map(String.init)
    return dirs.isEmpty ? nil : dirs
  }

  /// dirs에서 첫 번째로 실행 가능한 name을 찾는다(순수 — 존재 확인은 주입).
  public static func find(_ name: String, in dirs: [String], isExecutable: (String) -> Bool) -> String? {
    for d in dirs {
      let p = (d as NSString).appendingPathComponent(name)
      if isExecutable(p) { return p }
    }
    return nil
  }

  /// 로그인 셸(대화형 — nvm 등은 .zshrc에서 PATH를 바꾼다)에서 PATH를 받아 온다. 실패·시간 초과면 nil.
  public static func fromLoginShell(timeout: Double = 8) async -> [String]? {
    let shell = userShell()
    let runner = CommandRunner(searchPath: ["/bin", "/usr/bin"])
    let script = "printf '%s%s%s' '\(beginMarker)' \"$PATH\" '\(endMarker)'"
    guard let r = try? await runner.run(shell, ["-ilc", script], timeout: timeout, okCodes: [0, 1]) else { return nil }
    return parseMarked(r.stdout)
  }

  /// 앱이 쓸 최종 PATH: 로그인 셸 PATH(있으면) → 알려진 위치.
  public static func resolve(home: String = NSHomeDirectory()) async -> (dirs: [String], fromShell: Bool) {
    let shell = await fromLoginShell()
    return (merge(shell ?? [], knownDirs(home: home)), shell != nil)
  }

  static func userShell() -> String {
    if let pw = getpwuid(getuid()), let s = pw.pointee.pw_shell {
      let path = String(cString: s)
      if FileManager.default.isExecutableFile(atPath: path) { return path }
    }
    return "/bin/zsh"
  }
}

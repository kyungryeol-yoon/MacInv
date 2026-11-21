import AppKit

/// 실행 중인 앱의 캐시는 지우지 않는다 — Chromium은 캐시 파일을 연 채로 돈다(켜둔 채 지우면 오작동·이상한 재생성).
/// Notion·Slack을 켜둔 채 정리하는 게 가장 흔한 상황이라 실사용에서 바로 부딪힌다.
public enum Running {
  /// 지금 실행 중인 앱 번들 이름들. **조회 실패(빈 목록)는 "실행 중인 앱 없음"과 구분한다** —
  /// 예전엔 실패를 없음으로 처리해 보호가 조용히 꺼졌다. 안전장치가 조용히 사라지는 건 처음부터 없는 것보다 나쁘다.
  @MainActor public static func current() -> [String]? {
    let names = appNames(fromBundlePaths: NSWorkspace.shared.runningApplications.compactMap { $0.bundleURL?.path })
    return names.isEmpty ? nil : names   // 실행 중인 앱이 하나도 없을 수는 없다(Finder·Dock)
  }

  /// 번들 경로에서 `.app` 이름을 **전부** 뽑는다(순수). 헬퍼가 중첩되면(`Code.app/…/Code Helper.app`) 바깥 앱도 실행 중 —
  /// 놓쳐서 지우는 것보다 넉넉히 잡아 제외하는 쪽이 안전하다.
  public static func appNames(fromBundlePaths paths: [String]) -> [String] {
    var seen = Set<String>(), out: [String] = []
    for p in paths {
      for part in p.split(separator: "/") where part.lowercased().hasSuffix(".app") {
        let name = String(part.dropLast(4))
        if !name.isEmpty, seen.insert(name).inserted { out.append(name) }
      }
    }
    return out
  }

  /// 캐시 폴더 이름(Application Support/<이름>)과 다른 실행 중 앱 이름의 별칭.
  static let aliases: [String: [String]] = ["code": ["Visual Studio Code"], "code - insiders": ["Visual Studio Code - Insiders"], "chatgpt": ["ChatGPT"]]

  /// 폴더 이름이 실행 중인 앱인가(순수). ① 정규화 일치 ② 별칭 ③ 앱 이름을 **단어로** 쪼갰을 때 한 단어로 들어 있음
  /// ("Code" ⊂ "Visual Studio Code"). 부분 문자열이 아니라 단어 단위인 이유: "Home"·"Mail"이 무관한 앱에 걸리면
  /// 멀쩡한 캐시가 영영 정리되지 않는다. 그래도 애매하면 보호 쪽으로.
  public static func matches(dirName: String, running: [String]) -> Bool {
    let dir = key(dirName)
    guard !dir.isEmpty else { return false }
    let alias = (aliases[dirName.trimmingCharacters(in: .whitespaces).lowercased()] ?? []).map(key)
    for app in running {
      let a = key(app)
      if a == dir || alias.contains(a) { return true }
      if dir.count >= 3, words(app).contains(dir) { return true }
    }
    return false
  }

  static func key(_ s: String) -> String {
    var t = s.trimmingCharacters(in: .whitespaces)
    if t.lowercased().hasSuffix(".app") { t.removeLast(4) }
    return t.replacingOccurrences(of: " ", with: "").replacingOccurrences(of: "-", with: "").replacingOccurrences(of: "_", with: "").lowercased()
  }

  static func words(_ app: String) -> [String] {
    app.split(whereSeparator: { " _-.".contains($0) }).map { key(String($0)) }
  }
}

/// APFS 로컬 스냅샷 — "지워도 여유 공간이 안 느는 이유". **안내만** 한다(삭제는 백업 복원 지점을 없애는 일이라 도구가 결정하지 않는다).
public enum Snapshots {
  public static func parse(_ out: String) -> [String] {
    out.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { $0.hasPrefix("com.apple.TimeMachine.") }
  }

  public static func scan(runner: CommandRunner) async -> [String]? {
    guard let r = try? await runner.run("tmutil", ["listlocalsnapshots", "/"], timeout: 30) else { return nil }
    return parse(r.stdout)
  }
}

/// Ollama 모델 — 위치 통째로 지우지 않는다(모델 하나가 수 GB, 다시 받기 비쌈). 모델별로 보여 주고 하나씩 고르게.
public struct OllamaModel: Sendable, Hashable, Identifiable {
  public var id: String { name }
  public let name: String
  public let sizeKB: Int
}

public enum Ollama {
  /// `ollama list` — `NAME  ID  SIZE  MODIFIED` (예: `llama3.1:8b  46e0c10c039e  4.9 GB  3 weeks ago`)
  public static func parse(_ out: String) -> [OllamaModel] {
    let re = try! NSRegularExpression(pattern: #"^(\S+)\s+[0-9a-f]{6,}\s+([\d.]+)\s*(KB|MB|GB|TB)\b"#)
    return out.split(separator: "\n").compactMap { raw in
      let line = String(raw)
      guard let m = re.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)),
            let n = Range(m.range(at: 1), in: line), let v = Range(m.range(at: 2), in: line), let u = Range(m.range(at: 3), in: line),
            let num = Double(line[v]) else { return nil }
      let mult: Double = ["KB": 1, "MB": 1024, "GB": 1024 * 1024, "TB": 1024 * 1024 * 1024][String(line[u])] ?? 0
      return OllamaModel(name: String(line[n]), sizeKB: Int(num * mult))
    }
  }

  public static func scan(runner: CommandRunner) async -> [OllamaModel]? {
    guard let r = try? await runner.run("ollama", ["list"], timeout: 20) else { return nil }
    return parse(r.stdout)
  }
}

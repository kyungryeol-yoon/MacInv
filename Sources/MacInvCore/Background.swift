import Foundation
import Security

/// 백그라운드 항목(LaunchAgent/LaunchDaemon plist)과 그 주인.
/// 시스템 설정 「로그인 항목 → 백그라운드 앱 활동」에 보이는 것의 대부분이 여기서 나온다.
/// 앱을 지워도 plist가 남으면 목록에 계속 남는다(2026-11-21 Riot 사례) — 그래서 "어느 앱 것인지"를 판정한다.
public struct BackgroundItem: Sendable, Identifiable, Hashable {
  public enum Kind: String, Sendable { case userAgent, systemAgent, systemDaemon
    public var label: String {
      switch self { case .userAgent: return "사용자 에이전트"; case .systemAgent: return "시스템 에이전트"; case .systemDaemon: return "시스템 데몬" }
    }
  }
  public var id: String { file }
  public let file: String
  public let kind: Kind
  public let label: String?
  public let program: String?
  public let runAtLoad: Bool
  /// 실행 파일이 디스크에 없음 — 주인 앱이 지워진 강한 신호
  public let programMissing: Bool
  /// plist 내용이 비어 있음(`{}`) — 아무것도 실행하지 않는 껍데기(예: 구글이 옛 Keystone을 끄며 남긴 파일)
  public var isEmpty: Bool = false
  public var owner: BackgroundOwner
}

public enum BackgroundOwner: Sendable, Hashable {
  case app(String)            // 실행 파일이 이 앱 안에 있거나 이름(ID)이 같음
  case sameDeveloper(String)  // 서명 팀이 이 앱과 같음(예: LogiRightSight ↔ Logi Options+)
  case brew(String)
  case apple
  case unknown                // 설치된 어느 것과도 연결 안 됨 — 잔여물 후보

  public var label: String {
    switch self {
    case .app(let n): return n
    case .sameDeveloper(let n): return "\(n)와 같은 개발사"
    case .brew(let f): return "brew \(f)"
    case .apple: return "Apple"
    case .unknown: return "주인 없음"
    }
  }
}

/// 주인 판정에 쓰는 설치된 앱 정보.
public struct OwnerCandidate: Sendable, Hashable {
  public let name: String
  public let path: String
  public let bundleID: String?
  public let teamID: String?
  public init(name: String, path: String, bundleID: String?, teamID: String?) {
    self.name = name; self.path = path; self.bundleID = bundleID; self.teamID = teamID
  }
}

public enum Background {
  static func dirs(home: String) -> [(String, BackgroundItem.Kind)] {
    [("\(home)/Library/LaunchAgents", .userAgent), ("/Library/LaunchAgents", .systemAgent), ("/Library/LaunchDaemons", .systemDaemon)]
  }

  /// plist 딕셔너리 → (Label, 실행 파일, RunAtLoad). Program이 없으면 ProgramArguments[0](순수).
  public static func fields(_ plist: [String: Any]) -> (label: String?, program: String?, runAtLoad: Bool) {
    let program = (plist["Program"] as? String) ?? (plist["ProgramArguments"] as? [String])?.first
    return (plist["Label"] as? String, program, plist["RunAtLoad"] as? Bool ?? false)
  }

  /// 주인 판정(순수). 순서가 곧 확신도다.
  /// 1) Apple  2) 실행 파일이 앱 번들 안  3) Homebrew 경로  4) plist 이름·Label이 앱 Bundle ID와 같거나 그 하위(가장 긴 것)
  /// 5) 실행 파일의 서명 팀 = 앱의 서명 팀  6) 없음
  public static func owner(file: String, label: String?, program: String?, programTeam: String?,
                           apps: [OwnerCandidate]) -> BackgroundOwner {
    let stem = ((file as NSString).lastPathComponent as NSString).deletingPathExtension
    let ids = [stem, label].compactMap { $0?.lowercased() }
    if ids.contains(where: { $0.hasPrefix("com.apple.") }) { return .apple }

    if let program {
      if let a = apps.first(where: { program.hasPrefix($0.path + "/") }) { return .app(a.name) }
      if let f = brewFormula(program) { return .brew(f) }
    }

    var best: (OwnerCandidate, Int)?
    for a in apps {
      guard let bid = a.bundleID?.lowercased() else { continue }
      for id in ids where id == bid || id.hasPrefix(bid + ".") {
        if bid.count > (best?.1 ?? 0) { best = (a, bid.count) }
      }
    }
    if let best { return .app(best.0.name) }

    if let programTeam, let a = apps.first(where: { $0.teamID == programTeam }) { return .sameDeveloper(a.name) }
    return .unknown
  }

  /// `/opt/homebrew/opt/<포뮬러>/…`·`/opt/homebrew/Cellar/<포뮬러>/…`(인텔은 /usr/local) → 포뮬러 이름(순수).
  /// 접두사를 먼저 떼야 한다 — `/opt/homebrew/opt/ollama`에서 맨 앞 `opt`를 잡으면 "homebrew"가 나온다.
  public static func brewFormula(_ path: String) -> String? {
    for prefix in ["/opt/homebrew/", "/usr/local/"] where path.hasPrefix(prefix) {
      let segs = path.dropFirst(prefix.count).split(separator: "/").map(String.init)
      if segs.count >= 2, segs[0] == "opt" || segs[0] == "Cellar" { return segs[1] }
    }
    return nil
  }

  public static func scan(apps: [AppItem], home: String = NSHomeDirectory()) -> [BackgroundItem] {
    let fm = FileManager.default
    let candidates = apps.map { OwnerCandidate(name: $0.name, path: $0.path, bundleID: $0.bundleID, teamID: CodeSign.teamID($0.path)) }
    var out: [BackgroundItem] = []
    for (dir, kind) in dirs(home: home) {
      for name in ((try? fm.contentsOfDirectory(atPath: dir)) ?? []).sorted() where name.hasSuffix(".plist") {
        let file = "\(dir)/\(name)"
        guard let data = fm.contents(atPath: file),
              let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any] else {
          out.append(BackgroundItem(file: file, kind: kind, label: nil, program: nil, runAtLoad: false, programMissing: false,
                                    owner: owner(file: file, label: nil, program: nil, programTeam: nil, apps: candidates)))
          continue
        }
        if plist.isEmpty {
          out.append(BackgroundItem(file: file, kind: kind, label: nil, program: nil, runAtLoad: false, programMissing: false,
                                    isEmpty: true, owner: owner(file: file, label: nil, program: nil, programTeam: nil, apps: candidates)))
          continue
        }
        let f = fields(plist)
        let missing = f.program.map { !fm.fileExists(atPath: $0) } ?? false
        let team = (f.program != nil && !missing) ? CodeSign.teamID(f.program!) : nil
        out.append(BackgroundItem(file: file, kind: kind, label: f.label, program: f.program, runAtLoad: f.runAtLoad,
                                  programMissing: missing,
                                  owner: owner(file: file, label: f.label, program: f.program, programTeam: team, apps: candidates)))
      }
    }
    return out
  }
}

/// 코드 서명 정보(외부 명령 없이 Security 프레임워크).
public enum CodeSign {
  /// 서명 팀 ID(개발사). ad-hoc·서명 없음이면 nil.
  public static func teamID(_ path: String) -> String? {
    var code: SecStaticCode?
    guard SecStaticCodeCreateWithPath(URL(fileURLWithPath: path) as CFURL, [], &code) == errSecSuccess, let code else { return nil }
    var info: CFDictionary?
    guard SecCodeCopySigningInformation(code, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
          let d = info as? [String: Any] else { return nil }
    return d[kSecCodeInfoTeamIdentifier as String] as? String
  }
}

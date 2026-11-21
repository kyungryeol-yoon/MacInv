import Foundation

/// 정리 계획 — **읽기 전용.** 무엇을 어떻게 지울지만 담고, 실행은 6단계에서 명시적 승인 뒤에만.
/// 삭제는 rm이 아니라 휴지통(또는 네이티브 명령 — brew uninstall 등). 명령은 인자 배열로 담는다(문자열로 뒀다
/// 다시 쪼개면 공백 든 인자가 깨진다 — Go판 6-8). 표시용 한 줄은 `shellLine`이 그릴 때만 만든다.
public enum TargetKind: String, Sendable {
  case app, brewCask, brewFormula, npm, pipx, cargo, goBin, mas, orphan, disk, ollama
  public var label: String {
    switch self {
    case .mas: return "App Store"
    case .orphan: return "고아 잔여물"
    case .disk: return "디스크 정리"
    case .ollama: return "Ollama 모델"
    case .app: return "앱"
    case .brewCask: return "Homebrew 앱(cask)"
    case .brewFormula: return "Homebrew 포뮬러"
    case .npm: return "npm 전역"
    case .pipx: return "pipx"
    case .cargo: return "cargo"
    case .goBin: return "Go 실행 파일"
    }
  }
}

/// 휴지통으로 옮길 경로 하나.
public struct PlanPath: Sendable, Hashable, Identifiable {
  public var id: String { path }
  public let path: String
  public let confidence: Confidence
  public let reason: String
  /// 잰 용량(KB). 아직 안 쟀으면 nil
  public var sizeKB: Int?
  public init(_ m: PathMatch, sizeKB: Int? = nil) {
    path = m.path; confidence = m.confidence; reason = m.reason; self.sizeKB = sizeKB
  }
}

/// 휴지통으로 옮길 수 있나.
public enum TrashAccess: Sendable, Equatable {
  case ok
  /// 다른 사용자(root) 소유 — 관리자 암호가 필요하다. 자동으로 하지 않는다
  case needsAdmin
  /// 내 것인데 macOS가 막음(앱 관리·앱 데이터 보호) — MacInv에 권한을 주면 풀린다
  case needsPermission
}

/// 일부러 뺀 경로와 이유(다른 앱 소유·공유 컴포넌트 등) — 왜 안 지우는지 보여 준다.
public struct Exclusion: Sendable, Hashable, Identifiable {
  public var id: String { path }
  public let path: String
  public let reason: String
}

public struct PlanTarget: Sendable, Identifiable {
  public var id: String { "\(kind.rawValue):\(name)" }
  public let kind: TargetKind
  public let name: String
  public var bundleID: String?
  /// 실행할 네이티브 명령들(인자 배열)
  public var commands: [[String]] = []
  /// 명령 타임아웃(초). 삭제는 5분
  public var timeout: TimeInterval = 300
  public var trash: [PlanPath] = []
  /// 부모 폴더에 쓰기 권한이 없어 자동으로 못 옮기는 경로 — 실행·회수 예상에서 뺀다(Go판 6-10)
  public var needsAdmin: [PlanPath] = []
  /// 내 것인데 macOS 보호(앱 관리·앱 데이터)로 막힌 경로 — 시스템 설정에서 MacInv에 권한을 주면 풀린다
  public var needsPermission: [PlanPath] = []
  public var excluded: [Exclusion] = []
  public var warnings: [String] = []
  public var suggestions: [String] = []
  /// 이 대상이 지우는 앱 번들(앱·cask) — 실행 직전에 켜져 있는지 확인하는 데 쓴다.
  /// cask 번들은 brew가 지우므로 휴지통 목록(trash)에는 없어서 따로 둔다
  public var appBundles: [String] = []
  /// 명령이 비울 몫(휴지통 경로 밖) — 대상을 빼면 회수 예상에서도 빠지게 대상마다 둔다
  public var commandReclaimKB = 0
  public init(kind: TargetKind, name: String, bundleID: String? = nil) { self.kind = kind; self.name = name; self.bundleID = bundleID }
}

public struct Plan: Sendable {
  public var title: String
  public var verb = "삭제"
  public var targets: [PlanTarget] = []
  /// 계획 전체에 걸리는 경고(승인 전에 반드시 봐야 하는 것)
  public var warnings: [String] = []
  /// 계획을 만들지 않은 이유(자기 자신·애플 앱 등). targets가 비어 있을 때 보여 준다
  public var refusals: [String] = []
  /// 일부러 뺀 대상(런타임 메이저·sudo 필요 등) — 이유와 직접 실행할 명령을 보여 준다
  public var held: [HeldItem] = []
  /// 승인 뒤 한 번 더 확인(GUI 앱 삭제·재다운로드 비싼 것)
  public var requireConfirm = false
  /// 휴지통 경로 밖에서 회수될 몫(네이티브 명령이 지우는 것)
  public var extraReclaimKB = 0
  public init(title: String) { self.title = title }

  public var hasWork: Bool { targets.contains { !$0.commands.isEmpty || !$0.trash.isEmpty } }
  public var lowPaths: [String] { targets.flatMap(\.trash).filter { $0.confidence == .low }.map(\.path) }

  /// 실제로 담길 경로만 합한 회수 예상(KB). 확신낮음은 `includeLow`에 든 것만. 잰 값이 하나도 없으면 nil
  /// — 앱 목록의 표시 용량과 다를 수 있다(벤더 공유 보존 등, Go판 6-5)
  public func reclaimKB(includeLow: Set<String>) -> Int? {
    var kb = extraReclaimKB, any = extraReclaimKB > 0
    for p in targets.flatMap(\.trash) where p.confidence != .low || includeLow.contains(p.path) {
      if let s = p.sizeKB { kb += s; any = true }
    }
    return any ? kb : nil
  }

  /// 사용자가 뺀 대상(id)을 걷어낸 사본
  public func without(_ ids: Set<String>) -> Plan {
    var p = self
    p.targets.removeAll { ids.contains($0.id) }
    p.extraReclaimKB = 0   // 명령 몫은 대상별로 다시 셀 수 없어 뺀 게 있으면 0부터 — 아래에서 대상 몫만 다시 더함
    for t in p.targets { p.extraReclaimKB += t.commandReclaimKB }
    return p
  }

  /// 실행 직전 사본: 확신낮음은 고른 것만 남긴다(기본은 전부 뺌)
  public func selecting(low: Set<String>) -> Plan {
    var p = self
    for i in p.targets.indices { p.targets[i].trash.removeAll { $0.confidence == .low && !low.contains($0.path) } }
    return p
  }
}

public struct HeldItem: Sendable, Hashable, Identifiable {
  public var id: String { name }
  public let name: String
  public let reason: String
  public let command: [String]?
}

/// 표시용 셸 한 줄(실행에는 쓰지 않는다).
public func shellLine(_ argv: [String]) -> String {
  argv.map { a in
    let safe = a.unicodeScalars.allSatisfy { CharacterSet.alphanumerics.contains($0) || "-_./=:@+%,".unicodeScalars.contains($0) }
    return safe && !a.isEmpty ? a : "'" + a.replacingOccurrences(of: "'", with: "'\\''") + "'"
  }.joined(separator: " ")
}

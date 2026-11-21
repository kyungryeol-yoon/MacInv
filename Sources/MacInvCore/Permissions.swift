import Foundation

/// 개인정보 보호 권한 상태.
/// - 전체 디스크 접근: 다른 앱의 ~/Library/Containers·Group Containers·Mail·Safari를 읽고 지우는 데 필요.
///   없으면 용량이 **조용히 적게** 잡히므로 화면에 드러내야 한다.
/// - 확인 API가 없어서 보호된 경로를 실제로 열어 보고 판정한다.
/// - ad-hoc 서명이라 brew 업데이트 후 허용이 풀릴 수 있다 — 같은 방법으로 매번 다시 확인한다.
public enum Permissions {
  public enum Access: Sendable, Equatable { case granted, denied, unknown }

  /// 전체 디스크 접근이 있어야만 열리는 경로들(하나라도 열리면 허용).
  static func probes(home: String) -> [String] {
    ["/Library/Application Support/com.apple.TCC/TCC.db",
     "\(home)/Library/Safari/Bookmarks.plist",
     "\(home)/Library/Mail"]
  }

  /// 시험 결과(경로마다 열림 true / 막힘 false / 없음 nil)로 판정 — 순수 함수.
  public static func decide(_ results: [Bool?]) -> Access {
    if results.contains(true) { return .granted }
    if results.contains(false) { return .denied }
    return .unknown
  }

  public static func fullDiskAccess(home: String = NSHomeDirectory()) -> Access {
    let fm = FileManager.default
    let results: [Bool?] = probes(home: home).map { path in
      var isDir: ObjCBool = false
      guard fm.fileExists(atPath: path, isDirectory: &isDir) else { return nil }
      if isDir.boolValue { return (try? fm.contentsOfDirectory(atPath: path)) != nil }
      guard let h = FileHandle(forReadingAtPath: path) else { return false }
      defer { try? h.close() }
      return (try? h.read(upToCount: 1)) != nil
    }
    return decide(results)
  }

  /// 화면에 띄울 안내.
  public enum Notice: Sendable, Equatable {
    case none
    /// 한 번도 허용된 적 없음 — 켜는 방법 + 켠 뒤 다시 시작해야 적용
    case notGranted
    /// 전에는 허용됐는데 지금 막힘 — ad-hoc 서명이라 업데이트·다시 빌드 후 풀린 것. 목록에서 지웠다 다시 추가
    case lostAfterUpdate
  }

  /// 순수 함수: 지금 상태 + 전에 허용된 적이 있는지 → 안내.
  public static func notice(_ access: Access, grantedBefore: Bool) -> Notice {
    switch access {
    case .granted, .unknown: return .none
    case .denied: return grantedBefore ? .lostAfterUpdate : .notGranted
    }
  }

  /// 시스템 설정 → 개인정보 보호 및 보안 → 전체 디스크 접근 권한
  public static let fullDiskAccessSettingsURL = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles")!
}

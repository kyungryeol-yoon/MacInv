import Foundation

/// MacInv 자신.
/// brew로 깔면 MacInv는 cask 목록·/Applications/MacInv.app·자기 데이터 폴더·업데이트 대상에 동시에 나타난다.
/// 모든 정리 계획은 이 판정으로 자기 자신을 뺀다(실행 중 보호에 우연히 걸리는 것에 기대지 않는다).
public struct SelfIdentity: Sendable, Equatable {
  public static let defaultBundleID = "io.github.kyungryeol-yoon.macinv"
  public static let caskToken = "macinv"
  public static let appName = "MacInv"

  public let bundleID: String
  public let bundlePath: String
  public let home: String

  public init(bundleID: String = SelfIdentity.defaultBundleID, bundlePath: String, home: String) {
    self.bundleID = bundleID
    self.bundlePath = (bundlePath as NSString).standardizingPath
    self.home = home
  }

  /// 지금 실행 중인 앱 기준.
  public static func current() -> SelfIdentity {
    SelfIdentity(bundleID: Bundle.main.bundleIdentifier ?? defaultBundleID,
                 bundlePath: Bundle.main.bundlePath, home: NSHomeDirectory())
  }

  public func isSelf(bundleID other: String) -> Bool { other.caseInsensitiveCompare(bundleID) == .orderedSame }

  /// brew cask 이름(소문자만 쓰지만 대소문자 무시).
  public func isSelf(caskToken other: String) -> Bool { other.lowercased() == Self.caskToken }

  /// 경로가 MacInv 번들 안이거나 MacInv의 데이터인가.
  /// - 번들: 지금 실행 중인 번들 + 설치 위치 /Applications/MacInv.app (빌드 폴더에서 실행 중이어도 설치본을 지키게)
  /// - 데이터: 경로 마디 중 하나가 번들 ID이거나 `번들 ID.`으로 시작(.plist·.savedState 등), 또는 Application Support/MacInv
  public func isSelf(path: String) -> Bool {
    let p = (path as NSString).standardizingPath
    for b in [bundlePath, "/Applications/\(Self.appName).app"] where p == b || p.hasPrefix(b + "/") { return true }
    let appSupport = "\(home)/Library/Application Support/\(Self.appName)"
    if p == appSupport || p.hasPrefix(appSupport + "/") { return true }
    let id = bundleID.lowercased()
    return p.split(separator: "/").contains { part in
      let s = part.lowercased()
      return s == id || s.hasPrefix(id + ".")
    }
  }
}

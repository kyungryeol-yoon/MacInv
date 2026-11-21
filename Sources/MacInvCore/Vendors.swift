import Foundation

/// 벤더 폴더 표: 한 벤더의 앱들이 공유하는 ~/Library/…/<Vendor> 폴더를 어떻게 다룰지.
/// **새 벤더는 아래 JSON에 항목만 추가하면 된다**(코드 수정 없음). Go판 internal/paths/data/vendors.json과 같은 내용.
/// SwiftPM 리소스 번들은 .app 서명과 맞지 않아(번들 루트에 놓임) 문자열로 넣었다.
public struct Vendor: Sendable, Decodable, Hashable {
  public let vendor: String
  public let bundle_prefixes: [String]
  public let folders: [String]
  /// Bundle ID → 벤더 폴더 안 앱 하위 폴더 이름
  public let apps: [String: String]
  public let shared_components: [SharedComponent]
  public struct SharedComponent: Sendable, Decodable, Hashable {
    public let name: String
    /// 홈 기준 상대 경로. 끝의 `*`는 접두사 뜻
    public let globs: [String]
  }
}

public enum Vendors {
  public static let all: [Vendor] = {
    struct Table: Decodable { let vendors: [Vendor] }
    return (try? JSONDecoder().decode(Table.self, from: Data(json.utf8)).vendors) ?? []
  }()

  /// Bundle ID로 벤더와 앱 하위 폴더를 찾는다(순수). 표에 없으면 앱 이름에서 벤더명을 떼어 추정("Google Chrome" → "Chrome").
  public static func lookup(bundleID: String, appName: String, in vs: [Vendor] = all) -> (Vendor, String)? {
    for v in vs where v.bundle_prefixes.contains(where: { bundleID.hasPrefix($0) }) {
      return (v, v.apps[bundleID] ?? deriveSubdir(vendor: v.vendor, appName: appName))
    }
    return nil
  }

  static func deriveSubdir(vendor: String, appName: String) -> String {
    var name = appName
    if name.lowercased().hasSuffix(".app") { name.removeLast(4) }
    let trimmed = name.hasPrefix(vendor) ? String(name.dropFirst(vendor.count)).trimmingCharacters(in: .whitespaces) : name
    return trimmed.isEmpty ? name : trimmed
  }

  static let json = #"""
  {"vendors":[
   {"vendor":"Google","bundle_prefixes":["com.google."],"folders":["Google"],
    "apps":{"com.google.Chrome":"Chrome","com.google.Chrome.canary":"Chrome Canary","com.google.Chrome.beta":"Chrome Beta","com.google.drivefs":"DriveFS"},
    "shared_components":[{"name":"Keystone / GoogleUpdater (자동 업데이트)","globs":["Library/Google/GoogleSoftwareUpdate","Library/Google/GoogleUpdater","Library/LaunchAgents/com.google.keystone.*","Library/LaunchAgents/com.google.GoogleUpdater.*"]}]},
   {"vendor":"Microsoft","bundle_prefixes":["com.microsoft."],"folders":["Microsoft"],
    "apps":{"com.microsoft.Word":"Word","com.microsoft.Excel":"Excel","com.microsoft.Powerpoint":"PowerPoint","com.microsoft.Outlook":"Outlook","com.microsoft.onenote.mac":"OneNote","com.microsoft.VSCode":"Code"},
    "shared_components":[{"name":"Microsoft AutoUpdate (MAU, 공유 업데이터)","globs":["Library/Application Support/Microsoft AutoUpdate","Library/Caches/com.microsoft.autoupdate.fba","Library/LaunchAgents/com.microsoft.update.*","Library/LaunchAgents/com.microsoft.autoupdate.*","Library/Preferences/com.microsoft.autoupdate2.plist"]},
                         {"name":"Office 공유 데이터 / 라이선스 (Group Containers)","globs":["Library/Group Containers/UBF8T346G9.*","Library/Preferences/com.microsoft.office.plist"]}]},
   {"vendor":"Adobe","bundle_prefixes":["com.adobe."],"folders":["Adobe"],"apps":{},
    "shared_components":[{"name":"Adobe Creative Cloud / OOBE (공유)","globs":["Library/Application Support/Adobe/Adobe Desktop Common","Library/Application Support/Adobe/OOBE","Library/Application Support/Adobe/AdobeGCClient"]}]},
   {"vendor":"JetBrains","bundle_prefixes":["com.jetbrains."],"folders":["JetBrains"],
    "apps":{"com.jetbrains.intellij":"IntelliJIdea","com.jetbrains.pycharm":"PyCharm","com.jetbrains.goland":"GoLand","com.jetbrains.WebStorm":"WebStorm"},
    "shared_components":[{"name":"JetBrains Toolbox / 공유 설정","globs":["Library/Application Support/JetBrains/Toolbox"]}]}
  ]}
  """#
}

import Testing
import Foundation
@testable import MacInvCore

/// 가짜 파일 시스템: 폴더 → 항목 이름들. exists는 "어느 폴더의 항목이거나 폴더 자체".
func fakeFS(_ tree: [String: [String]]) -> FileSystem {
  let all = Set(tree.flatMap { dir, names in [dir] + names.map { "\(dir)/\($0)" } })
  return FileSystem(list: { tree[$0] ?? [] }, exists: { all.contains($0) })
}

@Suite("Paths")
struct PathsTests {
  let home = "/Users/me"

  /// 마디 경계(2026-11-21 점검): com.foo.Bar가 com.foo.BarTender(설치 안 된 다른 앱·도구)를 삼키면 안 된다.
  @Test func leftoversRespectSegmentBoundary() {
    let fs = fakeFS([
      "\(home)/Library/Preferences": ["com.foo.Bar.plist", "com.foo.Bar.helper.plist", "com.foo.BarTender.plist", "com.foo.Bar-old.plist"],
      "\(home)/Library/Caches": ["com.foo.Bar", "com.foo.BarTender"],
      "\(home)/Library/Saved Application State": ["com.foo.Bar.savedState", "com.foo.BarTender.savedState"],
      "\(home)/Library/Group Containers": ["ABCDE12345.com.foo.Bar", "group.com.foo.Bar.shared", "ABCDE12345.com.foo.BarTender", "xcom.foo.Bar"],
    ])
    let got = Set(Paths.leftovers(home: home, bundleID: "com.foo.Bar", appName: "", fs: fs).map { ($0.path as NSString).lastPathComponent })
    #expect(got == ["com.foo.Bar.plist", "com.foo.Bar.helper.plist", "com.foo.Bar", "com.foo.Bar.savedState",
                    "ABCDE12345.com.foo.Bar", "group.com.foo.Bar.shared"])
  }

  @Test(arguments: ["/System", "/System/Library/x", "/usr/bin/git", "/bin", "/sbin/x", "/Library/Apple/x", "/usr/../usr/lib"])
  func protected(_ p: String) { #expect(Paths.isProtected(p)) }

  @Test(arguments: ["/Library/Application Support/x", "/Users/me/Library/System", "/usrlocal", "/Applications/Safari.app"])
  func notProtected(_ p: String) { #expect(!Paths.isProtected(p)) }

  @Test func leftoversByBundleIDAndName() {
    let fs = fakeFS([
      "\(home)/Library/Preferences": ["com.x.App.plist", "com.x.App.helper.plist", "com.x.Other.plist", "com.x.App.txt"],
      "\(home)/Library/Caches": ["com.x.App", "com.x.App.ShipIt", "App"],
      "\(home)/Library/Saved Application State": ["com.x.App.savedState"],
      "\(home)/Library/Group Containers": ["ABCDE12345.com.x.App", "group.com.x.App.shared", "ABCDE12345.com.y.Z"],
      "/Library/LaunchDaemons": ["com.x.App.updater.plist"],
      "\(home)/Library/Application Support": ["App"],
    ])
    let got = Paths.leftovers(home: home, bundleID: "com.x.App", appName: "App", fs: fs)
    let high = Set(got.filter { $0.confidence == .high }.map(\.path))
    #expect(high == [
      "\(home)/Library/Preferences/com.x.App.plist", "\(home)/Library/Preferences/com.x.App.helper.plist",
      "\(home)/Library/Caches/com.x.App", "\(home)/Library/Caches/com.x.App.ShipIt",
      "\(home)/Library/Saved Application State/com.x.App.savedState",
      "\(home)/Library/Group Containers/ABCDE12345.com.x.App", "\(home)/Library/Group Containers/group.com.x.App.shared",
      "/Library/LaunchDaemons/com.x.App.updater.plist",
    ])
    let low = Set(got.filter { $0.confidence == .low }.map(\.path))
    #expect(low == ["\(home)/Library/Caches/App", "\(home)/Library/Application Support/App"])
  }

  @Test func appNameIsNotAPattern() {
    // Go판 6-2: "Foo [beta]"가 glob 문자 클래스로 해석돼 ~/Library/Caches/Foo b를 삼켰다
    let fs = fakeFS(["\(home)/Library/Caches": ["Foo b", "Foo [beta]x"], "\(home)/Library/Application Support": ["Photos", "PhotosX"]])
    #expect(Paths.leftovers(home: home, bundleID: nil, appName: "Foo [beta]", fs: fs).isEmpty)
    #expect(Paths.leftovers(home: home, bundleID: nil, appName: "Photos*", fs: fs).isEmpty)
    #expect(Paths.leftovers(home: home, bundleID: nil, appName: "../../etc", fs: fs).isEmpty)
  }

  @Test func bundleIDMetacharactersAreLiteral() {
    // 패턴이 아니라 글자 그대로 — [b]가 b를 잡지 않는다. 마디 경계 규칙으로 [b]x도 안 잡는다(다른 ID)
    let fs = fakeFS(["\(home)/Library/Caches": ["com.a.b", "com.a.[b]", "com.a.[b].helper", "com.a.[b]x", "com.a.?"]])
    let got = Paths.leftovers(home: home, bundleID: "com.a.[b]", appName: "", fs: fs).map(\.path)
    #expect(got == ["\(home)/Library/Caches/com.a.[b]", "\(home)/Library/Caches/com.a.[b].helper"])
  }

  @Test(arguments: [
    ("HUAQ24HBR6.dev.orbstack", "OrbStack", true),     // ID(dev.kdrag0n.MacVirt)로는 못 찾는 24 GB
    ("group.com.example.My-App", "My App", true),
    ("UBF8T346G9.Office", "Office", false),             // 마디가 하나뿐(팀 ID 뗀 뒤) — Bundle ID 꼴 아님
    ("ABCDE12345.com.apple.mail", "Mail", true),
    ("group.com.example.MailPlus", "Mail", false),       // 부분 일치 금지
    ("group.com.example.home", "Home Assistant", false),
    ("x.y", "", false),
  ] as [(String, String, Bool)])
  func groupContainer(_ folder: String, _ app: String, _ want: Bool) {
    #expect(Paths.groupContainerMatchesApp(folder, appName: app) == want)
  }

  @Test func stripTeamID() {
    #expect(Paths.stripTeamIDPrefix("HUAQ24HBR6.dev.orbstack") == "dev.orbstack")
    #expect(Paths.stripTeamIDPrefix("group.com.x") == "group.com.x")
    #expect(Paths.stripTeamIDPrefix("huaq24hbr6.dev.x") == "huaq24hbr6.dev.x")   // 팀 ID는 대문자·숫자
  }

  @Test func dedupBothDirections() {
    func m(_ p: String) -> PathMatch { PathMatch(path: p, confidence: .high, reason: "") }
    // 자식이 먼저, 부모가 나중에 와도 부모만 남는다(Go판 6-4에서 빠져 있던 방향)
    #expect(Paths.dedupContained([m("/a/b/c"), m("/a/b"), m("/a/bc"), m("/a/b/d"), m("/a/b")]).map(\.path) == ["/a/b", "/a/bc"])
    #expect(Paths.dedupContained([m("/a"), m("/a/x"), m("")]).map(\.path) == ["/a"])
  }

  @Test func sizePathsAddVendorAndGroupContainers() {
    let fs = fakeFS([
      "\(home)/Library/Caches": ["com.google.Chrome"],
      "\(home)/Library/Application Support": ["Google"],
      "\(home)/Library/Application Support/Google": ["Chrome", "GoogleUpdater"],
      "\(home)/Library/Group Containers": ["EQHXZ8M8AV.com.google.Chrome"],
    ])
    let got = Set(Paths.sizePaths(home: home, bundleID: "com.google.Chrome", appName: "Google Chrome", fs: fs).map(\.path))
    #expect(got.contains("\(home)/Library/Application Support/Google/Chrome"))       // 벤더 폴더 안 앱 데이터
    #expect(!got.contains("\(home)/Library/Application Support/Google/GoogleUpdater"))   // 공유 컴포넌트는 아님
    #expect(!got.contains("\(home)/Library/Application Support/Google"))             // 벤더 폴더 전체도 아님
    // 측정 규칙은 삭제 규칙을 포함한다
    let left = Set(Paths.leftovers(home: home, bundleID: "com.google.Chrome", appName: "Google Chrome", fs: fs).map(\.path))
    #expect(left.isSubset(of: got))
  }

  @Test func vendorLookup() {
    #expect(Vendors.all.map(\.vendor) == ["Google", "Microsoft", "Adobe", "JetBrains"])
    #expect(Vendors.lookup(bundleID: "com.microsoft.VSCode", appName: "Visual Studio Code")?.1 == "Code")
    #expect(Vendors.lookup(bundleID: "com.adobe.Photoshop", appName: "Adobe Photoshop 2026")?.1 == "Photoshop 2026")   // 표에 없으면 이름에서 추정
    #expect(Vendors.lookup(bundleID: "dev.kdrag0n.MacVirt", appName: "OrbStack") == nil)
  }

  @Test func deniedRootsGroupedByMeasuredPath() {
    let measured = ["/Users/me/Library/Containers/com.kakao.KakaoTalkMac", "/Users/me/Library/Caches/com.kakao.KakaoTalkMac"]
    let err = """
    du: /Users/me/Library/Containers/com.kakao.KakaoTalkMac/Data/Library/a: Operation not permitted
    du: /Users/me/Library/Containers/com.kakao.KakaoTalkMac/Data/Library/b: Operation not permitted
    du: /Users/me/Library/Containers/com.kakao.KakaoTalkMac: Operation not permitted
    du: /Users/me/Library/Caches/x: No such file or directory
    """
    #expect(DiskUsage.deniedRoots(stderr: err, measured: measured) == ["/Users/me/Library/Containers/com.kakao.KakaoTalkMac"])
    // 잰 경로 밖이면 그 경로 그대로
    #expect(DiskUsage.deniedRoots(stderr: "du: /other/p: Permission denied", measured: measured) == ["/other/p"])
  }

  @Test func duPartial() {
    #expect(DiskUsage.isPartial(stderr: "du: /Users/me/Library/Containers/x: Operation not permitted\n"))
    #expect(DiskUsage.isPartial(stderr: "du: /x: Permission denied"))
    #expect(!DiskUsage.isPartial(stderr: ""))
  }
}

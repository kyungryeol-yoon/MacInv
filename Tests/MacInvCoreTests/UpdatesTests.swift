import Testing
import Foundation
@testable import MacInvCore

@Suite("Updates")
struct UpdatesTests {
  @Test func brewOutdated() {
    // 이 맥의 실제 출력 모양(2026-11-21)
    let json = """
    {"formulae":[{"name":"node","installed_versions":["25.9.0_2"],"current_version":"26.1.0","pinned":false,"pinned_version":null},
                 {"name":"brotli","installed_versions":["1.2.0"],"current_version":"1.2.1","pinned":true,"pinned_version":"1.2.0"}],
     "casks":[{"name":"diswitch","installed_versions":["0.1.0"],"current_version":"0.1.1"}]}
    """
    let got = Parse.brewOutdated(Data(json.utf8))
    #expect(got.map(\.name) == ["node", "brotli", "diswitch"])
    #expect(got[0].current == "25.9.0_2" && got[0].latest == "26.1.0" && got[0].source == .brewFormula)
    #expect(got[1].pinned)
    #expect(got[2].source == .brewCask)
  }

  @Test func npmOutdated() {
    let json = #"{"npm":{"current":"11.12.1","wanted":"11.21.3","latest":"11.21.3","dependent":"global"},"eas-cli":{"current":"18.7.0","latest":"27.11.0"}}"#
    let got = Parse.npmOutdated(Data(json.utf8))
    #expect(got.map(\.name) == ["eas-cli", "npm"])
    #expect(got[1].current == "11.12.1" && got[1].latest == "11.21.3")
    #expect(Parse.npmOutdated(Data("".utf8)).isEmpty)   // 업데이트 없으면 빈 출력
  }

  @Test func masOutdated() {
    let got = Parse.masOutdated("497799835 Xcode (26.4.0 -> 26.4.1)\n1295203466 Microsoft Remote Desktop (10.9.7 -> 10.9.8)\n")
    #expect(got.map(\.name) == ["Xcode", "Microsoft Remote Desktop"])
    #expect(got[0].current == "26.4.0" && got[0].latest == "26.4.1" && got[0].masID == "497799835")
  }

  @Test(arguments: [
    ("node", "25.9.0_2", "26.1.0", true),
    ("node", "25.9.0", "25.10.0", false),      // 같은 사이클 패치
    ("node@22", "22.1.0", "24.0.0", false),    // 고정 포뮬러
    ("python", "3.13.1", "3.14.0", true),      // 2마디
    ("python", "3.13.1", "3.13.4", false),
    ("go", "1.26.5", "1.27.0", false),         // 1.x 안 — 의도된 결정, 뒤집지 말 것
    ("rust", "1.88.0", "1.89.0", false),
    ("go", "1.26.5", "2.0.0", true),
    ("ffmpeg", "7.1", "8.0", false),           // 런타임 아님
    ("node", "v25.1.0", "v26.0.0", true),
  ] as [(String, String, String, Bool)])
  func runtimeMajor(_ name: String, _ cur: String, _ latest: String, _ want: Bool) {
    #expect(Updates.runtimeMajorJump(name: name, current: cur, latest: latest) == want)
  }

  @Test func cyclePrefix() {
    #expect(Updates.cyclePrefix("25.9.0_2", parts: 1) == "25")
    #expect(Updates.cyclePrefix("3.13.1", parts: 2) == "3.13")
    #expect(Updates.cyclePrefix("3", parts: 2) == nil)
    #expect(Updates.cyclePrefix("r3222", parts: 1) == nil)
  }

  @Test func runtimeMajorOnlyForBrewFormula() {
    let npmNode = OutdatedItem(name: "node", source: .npm, current: "25.0.0", latest: "26.0.0", pinned: false, masID: nil)
    #expect(!npmNode.runtimeMajor)
  }
}

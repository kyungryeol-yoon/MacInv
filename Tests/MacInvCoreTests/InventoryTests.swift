import Testing
import Foundation
@testable import MacInvCore

@Suite("Parse")
struct ParseTests {
  @Test func brewInfoFormulaeAndCasks() throws {
    // 실제 출력 모양(2026-11-21 이 맥): cask 앱은 {"app": [...], "target": ...}
    let json = """
    {"formulae":[
      {"name":"git","dependencies":["pcre2","gettext"],"installed":[{"version":"2.53.0_1","installed_on_request":true,"installed_as_dependency":false}]},
      {"name":"gettext","dependencies":[],"installed":[{"version":"1.0","installed_on_request":false,"installed_as_dependency":true}]}],
     "casks":[
      {"token":"diswitch","installed":"0.1.1","artifacts":[{"uninstall":[{"quit":"io.github.kyungryeol-yoon.diswitch"}]},{"app":["DiSwitch.app"],"target":"/Applications/DiSwitch.app"}]},
      {"token":"renamed","installed":"1.0","artifacts":[{"app":[{"target":"Other Name.app"}]}]},
      {"token":"legacy","installed":"2.0","artifacts":[["Legacy.app"]]},
      {"token":"font-x","installed":"1","artifacts":[{"font":["x.ttf"]}]}]}
    """
    let p = try #require(Parse.brewInfo(Data(json.utf8)))
    #expect(p.formulae.map(\.name) == ["gettext", "git"])
    let git = try #require(p.formulae.first { $0.name == "git" })
    #expect(git.version == "2.53.0_1" && git.onRequest && git.dependencies == ["pcre2", "gettext"])
    #expect(p.formulae.first { $0.name == "gettext" }?.onRequest == false)
    let apps = Dictionary(uniqueKeysWithValues: p.casks.map { ($0.token, $0.appNames) })
    #expect(apps["diswitch"] == ["DiSwitch.app"])     // Go판은 이걸 못 읽었다
    #expect(apps["renamed"] == ["Other Name.app"])
    #expect(apps["legacy"] == ["Legacy.app"])
    #expect(apps["font-x"] == [])
  }

  @Test func brewInfoGarbage() { #expect(Parse.brewInfo(Data("not json".utf8)) == nil) }

  @Test func npmLs() {
    let json = #"{"name":"lib","dependencies":{"npm":{"version":"11.12.1","overridden":false},"eas-cli":{"version":"18.7.0"}}}"#
    let got = Parse.npmLs(Data(json.utf8))
    #expect(got.map(\.name) == ["eas-cli", "npm"])
    #expect(got.map(\.version) == ["18.7.0", "11.12.1"])
    #expect(Parse.npmLs(Data("{}".utf8)).isEmpty)
  }

  @Test func pipxList() {
    let json = #"{"venvs":{"httpie":{"metadata":{"main_package":{"package_version":"3.2.4"}}},"black":{"metadata":{"main_package":{"package_version":"24.1.0"}}}}}"#
    let got = Parse.pipxList(Data(json.utf8))
    #expect(got.map(\.name) == ["black", "httpie"])
    #expect(got.first?.version == "24.1.0")
  }

  @Test func cargoList() {
    let out = "ripgrep v14.1.0:\n    rg\ncargo-edit v0.12.2:\n    cargo-add\n    cargo-rm\nlocal-tool v0.1.0 (/Users/me/tool):\n    tool\n"
    let got = Parse.cargoList(out)
    #expect(got.map(\.name) == ["ripgrep", "cargo-edit", "local-tool"])
    #expect(got.map(\.version) == ["14.1.0", "0.12.2", "0.1.0"])
  }

  @Test func masList() {
    let out = "497799835  Xcode      (26.4.1)\n 1295203466 Microsoft Remote Desktop (10.9.8)\nNo installed apps found\n"
    let got = Parse.masList(out)
    #expect(got.map(\.name) == ["Xcode", "Microsoft Remote Desktop"])
    #expect(got.map(\.version) == ["26.4.1", "10.9.8"])
    #expect(got.first?.path == "497799835")
  }
}

@Suite("AppScanner")
struct AppScannerTests {
  /// 가짜 파일 시스템
  let fs: [String: [(name: String, isDir: Bool)]] = [
    "/Applications": [("Safari.app", true), ("Utilities", true), ("Adobe Photoshop 2026", true), ("Some.bundle", true), (".hidden", true), ("notes.txt", false)],
    "/Applications/Utilities": [("Terminal.app", true)],
    "/Applications/Adobe Photoshop 2026": [("Adobe Photoshop 2026.app", true), ("Plug-ins", true)],
    "/Applications/Some.bundle": [("Inside.app", true)],
    "/Users/me/Applications": [("Chrome Apps.localized", true)],
    "/Users/me/Applications/Chrome Apps.localized": [("Gmail.app", true)],
    "/Users/Shared": [("Riot Games", true), ("Previously Relocated Items", true)],
    "/Users/Shared/Riot Games": [("Riot Client.app", true), ("Metadata", true)],
  ]

  @Test func findsNestedAndSharedApps() {
    let got = AppScanner.candidates(roots: AppScanner.roots(home: "/Users/me")) { fs[$0] ?? [] }
    #expect(got == [
      "/Applications/Safari.app",
      "/Applications/Adobe Photoshop 2026/Adobe Photoshop 2026.app",   // 폴더 안 앱
      "/Applications/Utilities/Terminal.app",                         // 중복 없이 한 번
      "/Users/me/Applications/Chrome Apps.localized/Gmail.app",
      "/Users/Shared/Riot Games/Riot Client.app",                      // 앱 폴더 밖 본체(Riot 사례)
    ])
  }

  @Test func origin() {
    #expect(AppScanner.origin(bundleID: "io.github.kyungryeol-yoon.diswitch", hasMASReceipt: false, cask: "diswitch") == .brewCask("diswitch"))
    #expect(AppScanner.origin(bundleID: "com.apple.Xcode", hasMASReceipt: true, cask: nil) == .appStore)   // App Store에서 받은 애플 앱
    #expect(AppScanner.origin(bundleID: "com.apple.Safari", hasMASReceipt: false, cask: nil) == .apple)
    #expect(AppScanner.origin(bundleID: "com.google.Chrome", hasMASReceipt: false, cask: nil) == .other)
    #expect(AppScanner.origin(bundleID: nil, hasMASReceipt: false, cask: nil) == .other)
  }

  @Test func caskIndexUsesFileName() {
    let idx = AppScanner.caskIndex([BrewCask(token: "diswitch", version: "0.1.1", appNames: ["DiSwitch.app"]),
                                    BrewCask(token: "x", version: nil, appNames: ["Sub/Folder/X.app"])])
    #expect(idx == ["DiSwitch.app": "diswitch", "X.app": "x"])
  }

  @Test func duParse() {
    let out = "123\t/Applications/A.app\n45678\t/Applications/B C.app\ndu: /x: Permission denied\n"
    #expect(DiskUsage.parse(out) == ["/Applications/A.app": 123, "/Applications/B C.app": 45678])
  }
}

@Suite("BrewGraph")
struct BrewGraphTests {
  let fs = [
    BrewFormula(name: "ffmpeg", version: "8", onRequest: true, dependencies: ["brotli", "x264"]),
    BrewFormula(name: "freetype", version: "2", onRequest: false, dependencies: ["brotli"]),
    BrewFormula(name: "brotli", version: "1", onRequest: false, dependencies: []),
    BrewFormula(name: "x264", version: "r", onRequest: false, dependencies: []),
    BrewFormula(name: "leftover", version: "1", onRequest: false, dependencies: []),   // 쓰던 게 지워짐
    BrewFormula(name: "hugo", version: "0.1", onRequest: true, dependencies: []),
  ]

  @Test func dependents() {
    let d = BrewGraph.dependents(fs)
    #expect(d["brotli"] == ["ffmpeg", "freetype"])
    #expect(d["x264"] == ["ffmpeg"])
    #expect(d["hugo"] == nil)
  }

  @Test func unusedIsAutoremoveCandidates() {
    // freetype는 의존성으로 깔렸지만 이제 아무도 안 씀 → 대상. 직접 설치(hugo)는 대상 아님
    #expect(BrewGraph.unused(fs) == ["freetype", "leftover"])
  }
}

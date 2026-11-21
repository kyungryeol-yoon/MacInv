import Testing
import Foundation
@testable import MacInvCore

@Suite("Planner 5-2")
struct PlannerMoreTests {
  let home = "/Users/me"
  let me = SelfIdentity(bundlePath: "/x/MacInv.app", home: "/Users/me")
  func env(_ tree: [String: [String]] = [:], running: [String]? = []) -> Planner.Env {
    Planner.Env(home: home, fs: fakeFS(tree), access: { _ in .ok }, me: me, running: running)
  }
  func o(_ n: String, _ s: OutdatedItem.Source, _ c: String = "1.0", _ l: String = "1.1", pinned: Bool = false, mas: String? = nil) -> OutdatedItem {
    OutdatedItem(name: n, source: s, current: c, latest: l, pinned: pinned, masID: mas)
  }

  // MARK: 업데이트

  @Test func upgradeEverythingHoldsRiskyAndPutsSelfLast() {
    var inv = Inventory()
    inv.casks = [BrewCask(token: "macinv", version: "1", appNames: []), BrewCask(token: "zoom", version: "1", appNames: [], usesInstaller: true)]
    let items = [o("macinv", .brewCask), o("node", .brewFormula, "25.1.0", "26.0.0"), o("node", .npm), o("git", .brewFormula),
                 o("go", .brewFormula, "1.25.0", "1.26.0"), o("zoom", .brewCask), o("hugo", .brewFormula, pinned: true),
                 o("npm", .npm), o("Xcode", .mas, mas: "497799835"), o("Pages", .mas)]
    let p = Planner.upgrade(items, everything: true, inventory: inv, env: env())
    #expect(p.targets.map(\.name) == ["node", "git", "go", "npm", "Xcode", "macinv"])   // npm의 node는 런타임 메이저 아님, go 1.x도 아님
    #expect(p.targets.last?.commands == [["brew", "upgrade", "--cask", "macinv"]])
    #expect(Set(p.held.map(\.name)) == ["node", "zoom", "hugo", "Pages"])
    #expect(p.held.first { $0.name == "node" }?.command == ["brew", "upgrade", "node"])
    #expect(p.targets.first { $0.name == "npm" }?.commands == [["npx", "-y", "npm@latest", "install", "-g", "npm@latest"]])
    #expect(p.targets.first { $0.name == "Xcode" }?.commands == [["mas", "upgrade", "497799835"]])
    #expect(p.targets.allSatisfy { $0.timeout == 1800 })
  }

  @Test func upgradeChosenRuntimeMajorIsIncludedWithWarning() {
    let p = Planner.upgrade([o("node", .brewFormula, "25.1.0", "26.0.0")], everything: false, inventory: Inventory(), env: env())
    #expect(p.held.isEmpty && p.targets.count == 1 && p.targets[0].warnings.first?.contains("메이저") == true)
  }

  @Test func caskInstallerFlagParsed() throws {
    let json = #"{"formulae":[],"casks":[{"token":"zoom","installed":"6","artifacts":[{"pkg":["Zoom.pkg"]},{"uninstall":[{"pkgutil":"us.zoom"}]}]},{"token":"hum","installed":"1","artifacts":[{"app":["Hum.app"]}]}]}"#
    let c = try #require(Parse.brewInfo(Data(json.utf8))).casks
    #expect(c.map(\.usesInstaller) == [false, true])   // hum, zoom (정렬)
  }

  // MARK: 고아 잔여물

  @Test(arguments: [
    ("com.gone.app", true),
    ("Google", false),                                   // 이름 폴더 — 귀속 불명
    ("com.apple.Safari", false), ("group.com.apple.podcasts", false), ("systemgroup.com.x.y", false),
    ("HUAQ24HBR6.dev.orbstack", false), ("is.workflow.my.shortcuts", false), ("org.sparkle-project.Sparkle", false),
    ("com.installed.app.helper", false),                 // 설치 앱의 파생물
    ("com.installed", false),                            // 설치 앱의 상위 ID
    ("COM.INSTALLED.APP", false),                        // 대소문자 무시
    ("com.google.Keystone", false),                      // 같은 벤더 앱이 살아 있음
    ("com.microsoft.gone", true),                        // 그 벤더 앱이 없음
    ("homebrew.mxcl.postgresql@16", false),              // 깔린 포뮬러의 서비스
    ("homebrew.mxcl.redis", true),
    ("sh.brew.ollama", false),                           // 새 brew services 이름(실측)
    ("org.swift.swiftpm", false),                        // swift 툴체인(실측)
    ("com.logi.cp-dev-mgr", false),                      // 같은 개발자 앱(Logi Options+)이 있음(실측)
    ("io.github.someone.tool", true),                    // io.github는 세 마디로 개발자 구분
    ("io.github.kyungryeol-yoon.oldapp", false),
    ("org.m0k.transmission", true),
  ])
  func orphan(_ id: String, _ want: Bool) {
    let installed = ["com.installed.app", "com.google.Chrome", "com.logi.optionsplus", "io.github.kyungryeol-yoon.hum"]
    #expect(Planner.isOrphan(id, installed: installed, tools: ["postgresql@16", "ollama", "swift"]) == want)
  }

  @Test func orphanIDExtraction() {
    #expect(Planner.orphanID("com.x.y.plist", strip: ".plist") == "com.x.y")
    #expect(Planner.orphanID("com.x.y", strip: ".plist") == nil)
    #expect(Planner.orphanID("group.com.x.y", strip: "") == "com.x.y")
  }

  @Test func orphansPlanGroupsByIDAndSkipsSelf() {
    let lib = "\(home)/Library"
    var inv = Inventory()
    inv.apps = [AppItem(name: "Keep", path: "/Applications/Keep.app", bundleID: "com.keep.app", version: nil, dateAdded: nil)]
    let p = Planner.orphans(inventory: inv, env: env(["\(lib)/Preferences": ["com.gone.app.plist", "com.keep.app.plist", "io.github.kyungryeol-yoon.macinv.plist"],
                                                      "\(lib)/Caches": ["com.gone.app", "Google"], "\(lib)/Containers": ["com.gone.app.Extension"]]))
    #expect(p.targets.map(\.name) == ["com.gone.app"])
    #expect(Set(p.targets[0].trash.map(\.path)) == ["\(lib)/Preferences/com.gone.app.plist", "\(lib)/Caches/com.gone.app", "\(lib)/Containers/com.gone.app.Extension"])
    #expect(p.targets[0].trash.allSatisfy { $0.confidence == .medium })
    #expect(p.requireConfirm)
  }

  // MARK: 디스크

  func entry(_ id: String, _ paths: [String: Int], orphan: Bool = false) -> DiskEntry {
    var e = DiskEntry(location: Disk.locations.first { $0.id == id }!, paths: paths.keys.sorted())
    e.pathSizes = paths; e.orphan = orphan; return e
  }

  @Test func diskDefaultIsSafeAndArtifactOnly() {
    var r = DiskReport()
    r.entries = [entry("npm-cache", ["/n": 500]), entry("xcode-deriveddata", ["/d1": 100, "/d2": 50]), entry("docker-data", ["/k": 9000]),
                 entry("gradle-cache", ["/g": 70])]
    let p = Planner.diskCleanup(r, includeExpensive: false, env: env())
    #expect(p.targets.count == 2)                                         // gradle은 정리 방법이 없어 빠짐
    #expect(p.targets[0].commands.first?.first == "npm" && p.extraReclaimKB == 500)
    #expect(p.targets[1].trash.map(\.sizeKB) == [100, 50])                 // 경로마다 제 용량(N배 부풀림 방지)
    #expect(p.reclaimKB(includeLow: []) == 650 && !p.requireConfirm)
    let withExp = Planner.diskCleanup(r, includeExpensive: true, env: env())
    #expect(withExp.targets.count == 3 && withExp.requireConfirm)
  }

  @Test func diskNeverAndRunningAreNeverIncluded() {
    var e = entry("electron-cache", ["/a/Claude/Cache": 1000, "/a/Slack/Cache": 200])
    e.blockedByRunning = ["/a/Claude/Cache": "Claude"]
    var r = DiskReport(); r.entries = [e]
    let t = Planner.diskCleanup(r, includeExpensive: true, env: env()).targets[0]
    #expect(t.trash.map(\.path) == ["/a/Slack/Cache"] && t.excluded.map(\.path) == ["/a/Claude/Cache"])
    r.runningCheckFailed = true
    #expect(Planner.diskCleanup(r, includeExpensive: false, env: env()).warnings.count == 1)
  }

  @Test func orphanCacheIsTrashedNotCommanded() {
    var r = DiskReport(); r.entries = [entry("pip-cache", ["/Users/me/Library/Caches/pip": 300], orphan: true)]
    let t = Planner.diskCleanup(r, includeExpensive: false, env: env()).targets[0]
    #expect(t.commands.isEmpty && t.trash.map(\.path) == ["/Users/me/Library/Caches/pip"] && t.trash[0].confidence == .medium)
    #expect(t.warnings.first?.contains("고아") == true)
  }

  @Test func singleLocationExplainsWhyNot() {
    var r = DiskReport(); r.entries = [entry("ollama-models", ["/o": 1]), entry("gradle-cache", ["/g": 1])]
    #expect(Planner.diskCleanup(r, only: ["ollama-models"], includeExpensive: false, env: env()).refusals.first?.contains("비싼") == true)
    #expect(Planner.diskCleanup(r, only: ["gradle-cache"], includeExpensive: false, env: env()).refusals.isEmpty == false)
  }

  @Test func symlinkedPathIsNotTrashed() {
    var e = entry("xcode-deriveddata", ["/d": 0]); e.linkedTo = ["/d": "/Volumes/SSD/d"]
    var r = DiskReport(); r.entries = [e]
    let p = Planner.diskCleanup(r, includeExpensive: false, env: env())
    #expect(p.targets.first?.trash.isEmpty == true && p.targets.first?.excluded.count == 1)
  }

  // MARK: Ollama

  @Test func ollamaPerModel() {
    let p = Planner.ollama([OllamaModel(name: "gemma4:12b", sizeKB: 8_000_000)], externalVolume: "SSD")
    #expect(p.targets[0].commands == [["ollama", "rm", "gemma4:12b"]] && p.extraReclaimKB == 8_000_000)
    #expect(p.warnings.first?.contains("SSD") == true && p.requireConfirm)
  }
}

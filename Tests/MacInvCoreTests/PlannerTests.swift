import Testing
import Foundation
@testable import MacInvCore

@Suite("Planner")
struct PlannerTests {
  let home = "/Users/me"
  var lib: String { "\(home)/Library" }
  let me = SelfIdentity(bundlePath: "/Users/me/develop/MacInv/build/MacInv.app", home: "/Users/me")

  func env(_ tree: [String: [String]], denied: Set<String> = [], protected: Set<String> = [], running: [String]? = []) -> Planner.Env {
    Planner.Env(home: home, fs: fakeFS(tree), access: { denied.contains($0) ? .needsAdmin : protected.contains($0) ? .needsPermission : .ok },
                me: me, running: running)
  }
  func app(_ name: String, _ id: String?, origin: AppOrigin = .other, dir: String = "/Applications") -> AppItem {
    AppItem(name: name, path: "\(dir)/\(name).app", bundleID: id, version: nil, dateAdded: nil, origin: origin)
  }
  func inv(_ apps: [AppItem], formulae: [BrewFormula] = []) -> Inventory {
    var i = Inventory(); i.apps = apps; i.formulae = formulae; return i
  }
  func paths(_ p: Plan) -> [String: Confidence] {
    Dictionary(p.targets.flatMap(\.trash).map { ($0.path, $0.confidence) }, uniquingKeysWith: { a, _ in a })
  }

  @Test func derivedBundleID() {
    #expect(Paths.derivedBundleID("/x/com.google.Chrome.canary.plist") == "com.google.Chrome.canary")
    #expect(Paths.derivedBundleID("/x/ByHost/com.foo.bar.2CD92239-1A2B-3C4D-5E6F-7A8B9C0D1E2F.plist") == "com.foo.bar")
    #expect(Paths.derivedBundleID("/x/HUAQ24HBR6.group.dev.orbstack") == "dev.orbstack")
    #expect(Paths.derivedBundleID("/x/com.foo.App.savedState") == "com.foo.App")
  }

  /// Go판 6-1(데이터 손실 직전): Chrome 계획이 Canary 것을 [확실]로 잡았다. 파생 헬퍼는 남겨서 지운다.
  @Test func leftoversDoNotSwallowOtherApp() {
    let e = env(["\(lib)/Caches": ["com.google.Chrome", "com.google.Chrome.canary"],
                 "\(lib)/LaunchAgents": ["com.google.Chrome.helper.plist"]])
    let chrome = app("Google Chrome", "com.google.Chrome"), canary = app("Google Chrome Canary", "com.google.Chrome.canary")
    let p = Planner.removal(app: chrome, inventory: inv([chrome, canary]), env: e)
    let got = paths(p)
    #expect(got["\(lib)/Caches/com.google.Chrome"] == .high)
    #expect(got["\(lib)/LaunchAgents/com.google.Chrome.helper.plist"] == .high)
    #expect(got["\(lib)/Caches/com.google.Chrome.canary"] == nil)
    #expect(p.targets[0].excluded.map(\.path) == ["\(lib)/Caches/com.google.Chrome.canary"])
    #expect(p.targets[0].excluded[0].reason.contains("Google Chrome Canary"))
  }

  /// 둘 다 깔린 상태에서 Canary를 지울 때 Canary 파생물이 Chrome 것으로 오판되면 안 된다(가장 긴 ID가 이긴다).
  @Test func longestOwnerWins() {
    let all = [InstalledApp(id: "com.google.Chrome", name: "Chrome"), InstalledApp(id: "com.google.Chrome.canary", name: "Canary")]
    #expect(Planner.owner(of: "com.google.Chrome.canary.helper", installed: all)?.name == "Canary")
    #expect(Planner.owner(of: "com.google.Chromebook", installed: all) == nil)   // 마디 경계
    let (kept, excluded) = Planner.excludeOtherApps([PathMatch(path: "/c/com.google.Chrome.canary.helper", confidence: .high, reason: "")],
                                                    selfID: "com.google.Chrome.canary", installed: all)
    #expect(kept.count == 1 && excluded.isEmpty)
  }

  @Test func vendorSharedFolderKeepsOtherAppsAndUpdater() {
    let e = env(["\(lib)/Application Support": ["Google"], "\(lib)/Application Support/Google": ["Chrome", "DriveFS"],
                 "\(lib)/Google": ["GoogleSoftwareUpdate"]])
    let chrome = app("Google Chrome", "com.google.Chrome"), drive = app("Google Drive", "com.google.drivefs")
    let p = Planner.removal(app: chrome, inventory: inv([chrome, drive]), env: e)
    let got = paths(p)
    #expect(got["\(lib)/Application Support/Google/Chrome"] == .low)
    #expect(got["\(lib)/Application Support/Google"] == nil)
    #expect(got["\(lib)/Google/GoogleSoftwareUpdate"] == nil)
    #expect(p.targets[0].excluded.contains { $0.path == "\(lib)/Google/GoogleSoftwareUpdate" })
  }

  @Test func vendorSoleAppTakesWholeFolderAndSharedComponents() {
    let e = env(["\(lib)/Application Support": ["Google"], "\(lib)/Application Support/Google": ["Chrome"],
                 "\(lib)/LaunchAgents": ["com.google.keystone.agent.plist", "com.other.plist"]])
    let chrome = app("Google Chrome", "com.google.Chrome")
    let got = paths(Planner.removal(app: chrome, inventory: inv([chrome]), env: e))
    #expect(got["\(lib)/Application Support/Google"] == .medium)
    #expect(got["\(lib)/Application Support/Google/Chrome"] == nil)            // 부모에 포함
    #expect(got["\(lib)/LaunchAgents/com.google.keystone.agent.plist"] == .medium)
    #expect(got["\(lib)/LaunchAgents/com.other.plist"] == nil)
  }

  @Test func officeLicenseIsKeptWhileAnotherOfficeAppRemains() {
    let e = env(["\(lib)/Application Support/Microsoft": ["Word", "Excel"], "\(lib)/Group Containers": ["UBF8T346G9.Office"]])
    let word = app("Microsoft Word", "com.microsoft.Word"), excel = app("Microsoft Excel", "com.microsoft.Excel")
    let p = Planner.removal(app: word, inventory: inv([word, excel]), env: e)
    #expect(paths(p)["\(lib)/Group Containers/UBF8T346G9.Office"] == nil)
    #expect(p.targets[0].excluded.contains { $0.path == "\(lib)/Group Containers/UBF8T346G9.Office" })
  }

  /// 앱 이름은 패턴이 아니다(Go판 6-2): "Foo [beta]"가 "Foo b"를 삼키면 안 된다.
  @Test func appNameIsLiteral() {
    let e = env(["\(lib)/Application Support": ["Foo b", "Foo [beta]"]])
    let foo = app("Foo [beta]", "com.foo.beta")
    let got = paths(Planner.removal(app: foo, inventory: inv([foo]), env: e))
    #expect(got["\(lib)/Application Support/Foo [beta]"] == .low)
    #expect(got["\(lib)/Application Support/Foo b"] == nil)
  }

  @Test func refusesSelfAppleAndSystem() {
    let e = env([:])
    let installed = AppItem(name: "MacInv", path: "/Applications/MacInv.app", bundleID: "io.github.kyungryeol-yoon.macinv", version: nil, dateAdded: nil, origin: .brewCask("macinv"))
    for a in [installed, app("Safari", "com.apple.Safari", origin: .apple), app("Xcode", "com.apple.dt.Xcode", origin: .appStore),
              app("Thing", "com.x.thing", dir: "/System/Applications")] {
      let p = Planner.removal(app: a, inventory: inv([a]), env: e)
      #expect(p.targets.isEmpty && !p.refusals.isEmpty, "\(a.name)")
    }
    #expect(Planner.removal(cask: BrewCask(token: "macinv", version: nil, appNames: []), inventory: inv([]), env: e).targets.isEmpty)
  }

  /// MacInv의 데이터가 다른 앱 계획에 섞여 들어와도 마지막 관문에서 빠진다.
  @Test func selfDataIsNeverTrashed() {
    let e = env(["\(lib)/Caches": ["io.github.kyungryeol-yoon", "io.github.kyungryeol-yoon.macinv"]])
    let other = app("Odd", "io.github.kyungryeol-yoon")
    let p = Planner.removal(app: other, inventory: inv([other]), env: e)
    #expect(paths(p)["\(lib)/Caches/io.github.kyungryeol-yoon.macinv"] == nil)
    #expect(p.targets[0].excluded.contains { $0.path == "\(lib)/Caches/io.github.kyungryeol-yoon.macinv" })
  }

  @Test func caskAppUsesBrewAndTrashesLeftovers() {
    let e = env(["\(lib)/Preferences": ["io.github.kyungryeol-yoon.diswitch.plist"]])
    let ds = app("DiSwitch", "io.github.kyungryeol-yoon.diswitch", origin: .brewCask("diswitch"))
    let p = Planner.removal(app: ds, inventory: inv([ds]), env: e)
    #expect(p.targets.count == 1 && p.targets[0].kind == .brewCask)
    #expect(p.targets[0].commands == [["brew", "uninstall", "--cask", "diswitch"]])
    #expect(paths(p).keys.sorted() == ["\(lib)/Preferences/io.github.kyungryeol-yoon.diswitch.plist"])   // 번들은 brew가
    #expect(p.requireConfirm)
  }

  /// 권한 없는 경로는 미리 분리(Go판 6-10) — 실패할 걸 알면서 시도하지 않는다.
  @Test func unwritableGoesToNeedsAdmin() {
    let e = env(["/Library/LaunchDaemons": ["com.x.helper.plist"], "\(lib)/Caches": ["com.x"]], denied: ["/Library/LaunchDaemons/com.x.helper.plist"])
    let x = app("X", "com.x")
    let t = Planner.removal(app: x, inventory: inv([x]), env: e).targets[0]
    #expect(t.needsAdmin.map(\.path) == ["/Library/LaunchDaemons/com.x.helper.plist"])
    #expect(!t.trash.contains { $0.path.hasPrefix("/Library/LaunchDaemons") })
  }

  /// 내 소유인데 macOS가 막는 것(앱 관리 — 2026-11-21 Claude.app 실측)은 관리자와 따로: 암호로는 안 풀린다.
  @Test func protectedByMacOSIsSeparateFromAdmin() {
    let claude = app("Claude", "com.anthropic.claudefordesktop")
    let t = Planner.removal(app: claude, inventory: inv([claude]), env: env([:], protected: ["/Applications/Claude.app"])).targets[0]
    #expect(t.needsPermission.map(\.path) == ["/Applications/Claude.app"] && t.needsAdmin.isEmpty && t.trash.isEmpty)
  }

  /// Chrome 웹 앱(com.google.Chrome.app.…)은 "다른 Google 앱"이 아니다 — 2026-11-21 실측: 웹 앱 하나 때문에
  /// Chrome 계획이 Google 폴더를 공유 중으로 보고, 웹 앱 계획엔 Keystone 보존 줄이 붙었다.
  @Test func chromeWebAppIsNotAnotherVendorApp() {
    let e = env(["\(lib)/Application Support": ["Google"], "\(lib)/Application Support/Google": ["Chrome"],
                 "\(lib)/LaunchAgents": ["com.google.keystone.agent.plist"]])
    let chrome = app("Google Chrome", "com.google.Chrome")
    let web = app("Gmail", "com.google.Chrome.app.abcdef", dir: "\(home)/Applications/Chrome Apps.localized")
    let i = inv([chrome, web])
    #expect(paths(Planner.removal(app: chrome, inventory: i, env: e))["\(lib)/Application Support/Google"] == .medium)
    let w = Planner.removal(app: web, inventory: i, env: e).targets[0]
    #expect(w.excluded.isEmpty && w.trash.map(\.path) == [web.path])
  }

  @Test func runningAppIsWarned() {
    let x = app("X", "com.x")
    #expect(Planner.removal(app: x, inventory: inv([x]), env: env([:], running: ["X"])).targets[0].warnings.first?.contains("실행 중") == true)
    #expect(Planner.removal(app: x, inventory: inv([x]), env: env([:], running: nil)).targets[0].warnings.first?.contains("확인하지 못했습니다") == true)
    #expect(Planner.removal(app: x, inventory: inv([x]), env: env([:], running: ["Y"])).targets[0].warnings.isEmpty)
  }

  @Test func formulaWarnsAboutUsersAndRuntime() {
    var i = inv([], formulae: [BrewFormula(name: "node", version: "25", onRequest: true, dependencies: []),
                               BrewFormula(name: "yarn", version: "1", onRequest: true, dependencies: ["node"])])
    i.runtimes = [Runtime(name: "node", installed: true, version: "25.0.0",
                          origins: [RuntimeOrigin(path: "/opt/homebrew/bin/node", resolved: "/opt/homebrew/Cellar/node/25/bin/node", manager: .brew, package: "node")])]
    let t = Planner.removal(formula: i.formulae[0], inventory: i, env: env([:])).targets[0]
    #expect(t.commands == [["brew", "uninstall", "node"]])
    #expect(t.warnings.count == 2)
    #expect(t.warnings[0].contains("yarn") && t.warnings[1].contains("지금 쓰이는 node"))
  }

  @Test func packages() {
    let e = env([:])
    #expect(Planner.removal(package: PackageItem(name: "eas-cli", version: nil, manager: .npm, path: nil), inventory: inv([]), env: e)
      .targets[0].commands == [["npm", "uninstall", "-g", "eas-cli"]])
    #expect(Planner.removal(package: PackageItem(name: "npm", version: nil, manager: .npm, path: nil), inventory: inv([]), env: e).targets.isEmpty)
    let go = Planner.removal(package: PackageItem(name: "gopls", version: nil, manager: .go, path: "/Users/me/go/bin/gopls"), inventory: inv([]), env: e)
    #expect(go.targets[0].trash.map(\.path) == ["/Users/me/go/bin/gopls"] && go.targets[0].commands.isEmpty)
  }

  @Test func reclaimCountsOnlySelectedLow() {
    var p = Plan(title: "t")
    var t = PlanTarget(kind: .app, name: "a")
    t.trash = [PlanPath(PathMatch(path: "/a", confidence: .high, reason: ""), sizeKB: 100),
               PlanPath(PathMatch(path: "/b", confidence: .low, reason: ""), sizeKB: 50),
               PlanPath(PathMatch(path: "/c", confidence: .medium, reason: ""), sizeKB: nil)]
    p.targets = [t]
    #expect(p.reclaimKB(includeLow: []) == 100)
    #expect(p.reclaimKB(includeLow: ["/b"]) == 150)
    #expect(p.selecting(low: []).targets[0].trash.map(\.path) == ["/a", "/c"])
  }

  /// [확신낮음] 부모가 [확실] 자식을 삼키면 낮음을 안 골랐을 때 확실한 것까지 빠진다.
  @Test func dedupKeepsHigherConfidenceChild() {
    let got = Paths.dedupContainedPlan([PlanPath(PathMatch(path: "/a/b", confidence: .high, reason: "")),
                                        PlanPath(PathMatch(path: "/a", confidence: .low, reason: "")),
                                        PlanPath(PathMatch(path: "/c", confidence: .medium, reason: "")),
                                        PlanPath(PathMatch(path: "/c/d", confidence: .low, reason: ""))])
    #expect(got.map(\.path) == ["/a/b", "/a", "/c"])
  }

  @Test func shellLineQuotes() {
    #expect(shellLine(["brew", "uninstall", "--cask", "diswitch"]) == "brew uninstall --cask diswitch")
    #expect(shellLine(["docker", "builder", "prune", "--filter", "until=24h"]) == "docker builder prune --filter until=24h")
    #expect(shellLine(["trash", "/a b/it's"]) == "trash '/a b/it'\\''s'")
  }
}

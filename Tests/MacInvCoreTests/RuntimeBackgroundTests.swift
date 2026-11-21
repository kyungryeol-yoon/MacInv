import Testing
import Foundation
@testable import MacInvCore

@Suite("Runtimes")
struct RuntimesTests {
  @Test(arguments: [
    ("v25.9.0\n", nil, "25.9.0"),
    ("Python 3.12.13\n", nil, "3.12.13"),
    ("go version go1.26.5 darwin/arm64\n", nil, "1.26.5"),
    ("openjdk version \"21.0.4\" 2024-07-16\n", nil, "21.0.4"),
    // swift --version 첫 줄 swift-driver 버전을 집으면 안 된다
    ("swift-driver version: 1.168.6 Apple Swift version 6.4.2 (swiftlang-6.4.2.1.9)\n", #"Apple Swift version (\d+\.\d+(?:\.\d+)?)"#, "6.4.2"),
  ] as [(String, String?, String)])
  func extractVersion(_ out: String, _ pattern: String?, _ want: String) {
    #expect(Runtimes.extractVersion(out, pattern: pattern) == want)
  }

  @Test func extractVersionNone() {
    #expect(Runtimes.extractVersion("The operation couldn’t be completed. Unable to locate a Java Runtime.") == nil)
  }

  @Test(arguments: [
    ("/Users/me/.nvm/versions/node/v22.1.0/bin/node", Manager.nvm, nil),
    ("/Users/me/.local/share/mise/installs/python/3.12/bin/python3", .mise, nil),
    ("/Users/me/.pyenv/versions/3.11.9/bin/python3.11", .pyenv, nil),
    ("/opt/homebrew/Cellar/node/25.9.0_2/bin/node", .brew, "node"),
    ("/opt/homebrew/lib/node_modules/@scope/tool/bin/cli.js", .npm, "@scope/tool"),
    ("/Users/me/.local/pipx/venvs/httpie/bin/http", .pipx, "httpie"),
    ("/Users/me/.cargo/bin/rustc", .cargo, nil),
    ("/Users/me/go/bin/gopls", .go, nil),
    ("/usr/bin/python3", .system, nil),
    ("/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/swift", .system, nil),
    ("/usr/local/custom/bin/tool", .manual, nil),
  ] as [(String, Manager, String?)])
  func classify(_ path: String, _ m: Manager, _ pkg: String?) {
    let (got, gotPkg) = Runtimes.classify(path, home: "/Users/me")
    #expect(got == m)
    #expect(gotPkg == pkg)
  }

  @Test func javaStubIsNotInstalled() {
    // JDK 없는 맥의 /usr/bin/java는 "Unable to locate a Java Runtime"만 찍는 껍데기
    let stub = [RuntimeOrigin(path: "/usr/bin/java", resolved: "/usr/bin/java", manager: .system, package: nil)]
    #expect(!Runtimes.isInstalled(origins: stub, version: nil))
    #expect(Runtimes.isInstalled(origins: stub, version: "21.0.4"))
    #expect(!Runtimes.isInstalled(origins: [], version: nil))
    let brew = [RuntimeOrigin(path: "/opt/homebrew/bin/x", resolved: "/opt/homebrew/Cellar/x/1/bin/x", manager: .brew, package: "x")]
    #expect(Runtimes.isInstalled(origins: brew, version: nil))   // 버전을 못 읽어도 직접 깐 것은 설치됨
  }

  @Test func conflict() {
    let a = RuntimeOrigin(path: "/a/node", resolved: "/r1/node", manager: .nvm, package: nil)
    let b = RuntimeOrigin(path: "/b/node", resolved: "/r2/node", manager: .brew, package: "node")
    let a2 = RuntimeOrigin(path: "/c/node", resolved: "/r1/node", manager: .nvm, package: nil)
    #expect(Runtime(name: "node", installed: true, version: "1", origins: [a, b]).conflict)
    #expect(!Runtime(name: "node", installed: true, version: "1", origins: [a, a2]).conflict)   // 같은 실제 파일로 가는 링크 둘
  }
}

@Suite("Background")
struct BackgroundTests {
  let apps = [
    OwnerCandidate(name: "logioptionsplus", path: "/Applications/logioptionsplus.app", bundleID: "com.logi.optionsplus", teamID: "QED4VVPZWA"),
    OwnerCandidate(name: "Google Chrome", path: "/Applications/Google Chrome.app", bundleID: "com.google.Chrome", teamID: "EQHXZ8M8AV"),
    OwnerCandidate(name: "OrbStack", path: "/Applications/OrbStack.app", bundleID: "dev.kdrag0n.MacVirt", teamID: "HUAQ24HBR6"),
    OwnerCandidate(name: "Riot Client", path: "/Users/Shared/Riot Games/Riot Client.app", bundleID: "com.riotgames.RiotGames.RiotClient", teamID: "K832E2UXV7"),
  ]

  @Test func fieldsPrefersProgramThenArguments() {
    #expect(Background.fields(["Label": "x", "Program": "/p", "ProgramArguments": ["/q"], "RunAtLoad": true]) == ("x", "/p", true))
    let f = Background.fields(["ProgramArguments": ["/opt/homebrew/opt/ollama/bin/ollama", "serve"]])
    #expect(f.label == nil && f.program == "/opt/homebrew/opt/ollama/bin/ollama" && !f.runAtLoad)
  }

  @Test func brewFormula() {
    #expect(Background.brewFormula("/opt/homebrew/opt/ollama/bin/ollama") == "ollama")   // 맨 앞 opt를 잡으면 "homebrew"
    #expect(Background.brewFormula("/opt/homebrew/Cellar/node/25.9.0/bin/node") == "node")
    #expect(Background.brewFormula("/usr/local/opt/redis/bin/redis-server") == "redis")
    #expect(Background.brewFormula("/opt/homebrew/bin/ollama") == nil)
    #expect(Background.brewFormula("/Library/PrivilegedHelperTools/x") == nil)
  }

  @Test func ownerProgramInsideApp() {
    let o = Background.owner(file: "/Users/me/Library/LaunchAgents/com.riot.riotclient.checkinstalls.plist", label: nil,
                             program: "/Users/Shared/Riot Games/Riot Client.app/Contents/MacOS/RiotClientServices", programTeam: nil, apps: apps)
    #expect(o == .app("Riot Client"))
  }

  @Test func ownerByFileNameMatchingBundleID() {
    // Options+ 에이전트: Label은 com.logi.cp-dev-mgr지만 plist 이름이 앱 ID와 같다
    let o = Background.owner(file: "/Library/LaunchAgents/com.logi.optionsplus.plist", label: "com.logi.cp-dev-mgr",
                             program: "/Library/Application Support/Logitech.localized/LogiOptionsPlus/logioptionsplus_agent.app/Contents/MacOS/logioptionsplus_agent",
                             programTeam: "QED4VVPZWA", apps: apps)
    #expect(o == .app("logioptionsplus"))
  }

  @Test func ownerBySameTeam() {
    // RightSight: 이름도 ID도 다르지만 Options+와 같은 팀이 서명
    let o = Background.owner(file: "/Library/LaunchAgents/com.logitech.LogiRightSight.Agent.plist", label: "com.logitech.LogiRightSight.Agent",
                             program: "/Library/Application Support/Logitech.localized/LogiRightSightForWebcams/LogiRightSight.app/Contents/MacOS/LogiRightSight",
                             programTeam: "QED4VVPZWA", apps: apps)
    #expect(o == .sameDeveloper("logioptionsplus"))
    // GoogleUpdater ↔ Chrome
    #expect(Background.owner(file: "/u/LaunchAgents/com.google.GoogleUpdater.wake.plist", label: "com.google.GoogleUpdater.wake",
                             program: "/u/GoogleUpdater.app/Contents/MacOS/GoogleUpdater", programTeam: "EQHXZ8M8AV", apps: apps) == .sameDeveloper("Google Chrome"))
  }

  @Test func ownerBrewAndApple() {
    #expect(Background.owner(file: "/u/sh.brew.ollama.plist", label: "sh.brew.ollama", program: "/opt/homebrew/opt/ollama/bin/ollama",
                             programTeam: nil, apps: apps) == .brew("ollama"))
    #expect(Background.owner(file: "/Library/LaunchDaemons/com.apple.x.plist", label: "com.apple.x", program: "/usr/libexec/x",
                             programTeam: nil, apps: apps) == .apple)
  }

  @Test func ownerUnknownWhenAppGone() {
    // 앱을 지웠더니 남은 에이전트: 실행 파일도 없고 팀도 모름
    let gone = apps.filter { $0.name != "Riot Client" }
    #expect(Background.owner(file: "/u/com.riot.riotclient.checkinstalls.plist", label: nil,
                             program: "/Users/Shared/Riot Games/Riot Client.app/Contents/MacOS/RiotClientServices",
                             programTeam: nil, apps: gone) == .unknown)
  }

  @Test func ownerPrefersLongestBundleID() {
    let two = [OwnerCandidate(name: "Chrome", path: "/A/Chrome.app", bundleID: "com.google.Chrome", teamID: nil),
               OwnerCandidate(name: "Chrome Canary", path: "/A/Canary.app", bundleID: "com.google.Chrome.canary", teamID: nil)]
    #expect(Background.owner(file: "/u/com.google.Chrome.canary.helper.plist", label: nil, program: nil, programTeam: nil, apps: two) == .app("Chrome Canary"))
    #expect(Background.owner(file: "/u/com.google.Chrome.helper.plist", label: nil, program: nil, programTeam: nil, apps: two) == .app("Chrome"))
  }
}

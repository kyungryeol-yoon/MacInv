import Testing
import Foundation
@testable import MacInvCore

@Suite("Running · Snapshots · Ollama")
struct RunningTests {
  @Test func appNamesIncludeOuterAppOfHelpers() {
    let got = Running.appNames(fromBundlePaths: [
      "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Renderer).app",
      "/Applications/Notion.app", "/System/Library/CoreServices/Finder.app", "/Applications/Notion.app",
    ])
    #expect(got == ["Visual Studio Code", "Code Helper (Renderer)", "Notion", "Finder"])
  }

  @Test(arguments: [
    ("Notion", ["Notion"], true),
    ("Code", ["Visual Studio Code"], true),                 // 별칭 + 단어
    ("logioptionsplus_agent", ["LogiOptionsPlus"], false),  // 폴더 이름이 앱 이름의 한 단어가 아님
    ("LogiOptionsPlus", ["logioptionsplus_agent"], true),   // 앱 이름을 단어로 쪼개면 들어 있음
    ("Home", ["Home Assistant Companion"], true),
    ("Home", ["Homebrew Helper"], false),                   // 부분 문자열 금지
    ("Mail", ["MailMate"], false),
    ("ChatGPT", ["ChatGPT"], true),
    ("Claude", [], false),
    ("", ["Anything"], false),
  ] as [(String, [String], Bool)])
  func matches(_ dir: String, _ running: [String], _ want: Bool) {
    #expect(Running.matches(dirName: dir, running: running) == want)
  }

  @Test func blockedOnlyForAppSupportGroups() {
    let home = "/Users/me"
    let ps = ["\(home)/Library/Application Support/Claude/Cache", "\(home)/Library/Application Support/Code/Code Cache",
              "\(home)/Library/Application Support/Slack/Partitions/ws/Cache"]
    let got = Disk.blocked(home: home, groupBy: Disk.groupByAppSupportDir, paths: ps, running: ["Claude", "Visual Studio Code"])
    #expect(got == [ps[0]: "Claude", ps[1]: "Code"])
    #expect(Disk.blocked(home: home, groupBy: nil, paths: ps, running: ["Claude"]).isEmpty)
  }

  @Test func reclaimableExcludesRunning() {
    let l = Disk.locations.first { $0.id == "electron-cache" }!
    var e = DiskEntry(location: l, paths: ["/a/Claude/Cache", "/a/Slack/Cache"])
    e.pathSizes = ["/a/Claude/Cache": 1000, "/a/Slack/Cache": 200]
    e.blockedByRunning = ["/a/Claude/Cache": "Claude"]
    var r = DiskReport(); r.entries = [e]
    #expect(r.reclaimableKB == 200)
    #expect(r.blockedByApp.map(\.name) == ["Claude"] && r.blockedByApp.first?.kb == 1000)
  }

  @Test func externalVolume() {
    #expect(Disk.externalVolume("/Volumes/Realtek RTL9210 NVME Media/Ollama/models") == "Realtek RTL9210 NVME Media")
    #expect(Disk.externalVolume("/Users/me/.ollama/models") == nil)
  }

  @Test func snapshotsParse() {
    let out = "Snapshots for disk /:\ncom.apple.TimeMachine.2026-11-20-101010.local\ncom.apple.TimeMachine.2026-11-21-090000.local\n"
    #expect(Snapshots.parse(out).count == 2)
    #expect(Snapshots.parse("Snapshots for disk /:\n").isEmpty)   // 이 맥(2026-11-21): 없음
  }

  @Test func ollamaParseRealOutput() {
    // 이 맥의 실제 출력(2026-11-21)
    let out = """
    NAME          ID              SIZE      MODIFIED
    gemma4:12b    c7597fc90b86    7.7 GB    12 days ago
    qwen3.5:9b    d63f830f28d5    7.6 GB    12 days ago
    tiny:latest   0123456789ab    512 MB    1 hour ago
    """
    let got = Ollama.parse(out)
    #expect(got.map(\.name) == ["gemma4:12b", "qwen3.5:9b", "tiny:latest"])
    #expect(got[0].sizeKB == Int(7.7 * 1024 * 1024) && got[2].sizeKB == 512 * 1024)
  }
}

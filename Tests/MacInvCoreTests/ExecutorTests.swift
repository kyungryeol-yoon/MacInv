import Testing
import Foundation
@testable import MacInvCore

/// 무엇을 실행·이동했는지 기록하는 가짜.
final class Recorder: @unchecked Sendable {
  private let lock = NSLock()
  private(set) var ran: [[String]] = []
  private(set) var trashed: [String] = []
  func ran(_ a: [String]) { lock.withLock { ran.append(a) } }
  func trashed(_ p: String) { lock.withLock { trashed.append(p) } }
}

@Suite("Executor")
struct ExecutorTests {
  let me = SelfIdentity(bundlePath: "/x/MacInv.app", home: "/Users/me")

  func exec(_ rec: Recorder, failing: Set<String> = [], existing: Set<String>? = nil, running: [String: String] = [:]) -> Executor {
    Executor(run: { argv, _ in
      rec.ran(argv)
      if failing.contains(argv.joined(separator: " ")) { throw CommandError.failed(command: argv.joined(separator: " "), exitCode: 1, detail: "Error: boom") }
      return CommandResult(stdout: "", stderr: "", exitCode: 0)
    }, trash: { rec.trashed($0) }, exists: { existing?.contains($0) ?? true }, runningOwner: { running[$0] }, me: me)
  }
  func target(_ kind: TargetKind, _ name: String, cmds: [[String]] = [], paths: [(String, Confidence)] = []) -> PlanTarget {
    var t = PlanTarget(kind: kind, name: name)
    t.commands = cmds
    t.trash = paths.map { PlanPath(PathMatch(path: $0.0, confidence: $0.1, reason: "")) }
    return t
  }
  func plan(_ ts: [PlanTarget]) -> Plan { var p = Plan(title: "t"); p.targets = ts; return p }

  @Test func runsCommandsThenTrashesAndSkipsUnchosenLow() async {
    let rec = Recorder()
    let p = plan([target(.brewCask, "x", cmds: [["brew", "uninstall", "--cask", "x"]],
                         paths: [("/Users/me/Library/Preferences/x.plist", .high), ("/Users/me/Library/Caches/X", .low), ("/Users/me/Library/Logs/X", .low)])])
    let s = await exec(rec).execute(p, low: ["/Users/me/Library/Logs/X"]) { _ in }
    #expect(rec.ran == [["brew", "uninstall", "--cask", "x"]])
    #expect(rec.trashed == ["/Users/me/Library/Preferences/x.plist", "/Users/me/Library/Logs/X"])
    #expect(s == ExecSummary(ok: 3, failed: 0, skipped: 0, trashed: 2))
  }

  /// brew uninstall이 실패했는데 설정만 지우면 앱은 남고 설정만 사라진다.
  @Test func failedCommandKeepsLeftovers() async {
    let rec = Recorder()
    let p = plan([target(.brewCask, "x", cmds: [["brew", "uninstall", "--cask", "x"]], paths: [("/Users/me/Library/Preferences/x.plist", .high)]),
                  target(.npm, "y", cmds: [["npm", "uninstall", "-g", "y"]])])
    var lines: [ExecLine] = []
    let box = LockedLines()
    let s = await exec(rec, failing: ["brew uninstall --cask x"]).execute(p, low: []) { await box.add($0) }
    lines = await box.all
    #expect(rec.trashed.isEmpty && rec.ran.count == 2)                 // 다음 대상(npm)은 계속
    #expect(s.failed == 1 && s.skipped == 1 && s.ok == 1)
    #expect(lines.last { $0.state == .failed }?.detail?.contains("boom") == true)
  }

  @Test func finalGateRefusesProtectedSelfAndRoots() async {
    let rec = Recorder()
    let p = plan([target(.disk, "d", paths: [("/System/Library/x", .high), ("/Users/me/Library/Caches/io.github.kyungryeol-yoon.macinv", .high),
                                             ("/Users/me/Library", .high), ("/Applications", .high), ("relative/x", .high), ("/Users/me/Library/Caches/ok", .high)])])
    let s = await exec(rec).execute(p, low: []) { _ in }
    #expect(rec.trashed == ["/Users/me/Library/Caches/ok"])
    #expect(s.failed == 5)
  }

  @Test func runningAppAndMissingPathAreSkipped() async {
    let rec = Recorder()
    let app = target(.app, "Claude", paths: [("/Applications/Claude.app", .high), ("/Users/me/Library/Caches/com.anthropic", .high)])
    let cache = target(.disk, "electron", paths: [("/Users/me/Library/Application Support/Slack/Cache", .high), ("/gone", .high)])
    let s = await exec(rec, existing: ["/Applications/Claude.app", "/Users/me/Library/Caches/com.anthropic", "/Users/me/Library/Application Support/Slack/Cache"],
                       running: ["/Applications/Claude.app": "Claude", "/Users/me/Library/Application Support/Slack/Cache": "Slack"])
      .execute(plan([app, cache]), low: []) { _ in }
    #expect(rec.trashed.isEmpty)
    #expect(s.skipped == 3 && s.failed == 0)
  }

  @Test func selfUpgradeIsFlagged() async {
    let rec = Recorder()
    let s = await exec(rec).execute(plan([target(.brewCask, "macinv", cmds: [["brew", "upgrade", "--cask", "macinv"]])]), low: []) { _ in }
    #expect(s.selfUpdated)
  }

  @Test func realTrashMovesFileAndKeepsItRecoverable() throws {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("macinv-exec-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    let f = dir.appendingPathComponent("x.txt"); try Data("x".utf8).write(to: f)
    var out: NSURL?
    try FileManager.default.trashItem(at: f, resultingItemURL: &out)
    #expect(!FileManager.default.fileExists(atPath: f.path))
    if let u = out as URL? { #expect(FileManager.default.fileExists(atPath: u.path)); try? FileManager.default.removeItem(at: u) }
    try? FileManager.default.removeItem(at: dir)
  }
}

actor LockedLines {
  var all: [ExecLine] = []
  func add(_ l: ExecLine) {
    if let i = all.firstIndex(where: { $0.id == l.id }) { all[i] = l } else { all.append(l) }
  }
}

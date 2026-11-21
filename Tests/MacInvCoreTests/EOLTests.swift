import Testing
import Foundation
@testable import MacInvCore

@Suite("EOL")
struct EOLTests {
  let now = EOL.dateFormatter.date(from: "2026-11-21")!
  func c(_ cycle: String, _ latest: String, _ eol: EOLCycle.EOLValue) -> EOLCycle { EOLCycle(cycle: cycle, latest: latest, eol: eol) }

  @Test func parseRealShape() throws {
    // endoflife.date 실제 모양(2026-11-21): eol은 날짜 문자열 또는 false, cycle은 문자열(숫자일 수도 있게 대비)
    let json = #"[{"cycle":"1.27","eol":false,"latest":"1.27.4"},{"cycle":"1.20","eol":true,"latest":"1.20.14"},{"cycle":26,"eol":"2029-04-30","latest":"26.13.1"}]"#
    let got = try #require(EOL.parse(Data(json.utf8)))
    #expect(got == [c("1.27", "1.27.4", .rolling), c("1.20", "1.20.14", .ended), c("26", "26.13.1", .date("2029-04-30"))])
    #expect(EOL.parse(Data("<html>".utf8)) == nil)
  }

  @Test func matchCycleLongestPrefix() {
    let go = [c("1.27", "", .rolling), c("1.26", "", .rolling), c("1.2", "", .ended)]
    #expect(EOL.matchCycle("go version go1.26.5 darwin/arm64", go)?.cycle == "1.26")   // "1.2"가 집으면 안 됨
    #expect(EOL.matchCycle("1.2.1", go)?.cycle == "1.2")
    let py = [c("3.14", "", .unknown), c("3.9", "", .unknown)]
    #expect(EOL.matchCycle("Python 3.9.6", py)?.cycle == "3.9")
    #expect(EOL.matchCycle("3.91.0", py) == nil)                                        // 3.9 사이클 아님
    #expect(EOL.matchCycle("v25.9.0_2", [c("26", "", .unknown), c("25", "", .unknown)])?.cycle == "25")
    #expect(EOL.matchCycle("", go) == nil)
  }

  @Test func statusByDate() {
    #expect(EOL.status(.date("2025-10-07"), now: now) == ("2025-10-07", .passed))
    #expect(EOL.status(.date("2027-04-30"), now: now) == ("2027-04-30", .soon))      // 1년 안
    #expect(EOL.status(.date("2029-04-30"), now: now) == ("2029-04-30", .supported))
    #expect(EOL.status(.rolling, now: now) == (nil, .rolling))
    #expect(EOL.status(.ended, now: now) == (nil, .passed))
    #expect(EOL.status(.date("garbage"), now: now) == (nil, .unknown))
  }

  @Test func compareThreeWays() {
    #expect(EOL.compare(installed: "25.9.0", latestInCycle: "25.9.0", newestCycle: "26", myCycle: "25") == .major)
    #expect(EOL.compare(installed: "3.12.1", latestInCycle: "3.12.13", newestCycle: "3.12", myCycle: "3.12") == .patch)
    #expect(EOL.compare(installed: "3.12.13", latestInCycle: "3.12.13", newestCycle: "3.12", myCycle: "3.12") == .current)
    #expect(EOL.compare(installed: "1.26.10", latestInCycle: "1.26.9", newestCycle: "1.26", myCycle: "1.26") == .current)   // 10 > 9(문자열 비교면 틀림)
    #expect(EOL.compare(installed: "", latestInCycle: "1", newestCycle: "1", myCycle: "1") == .unknown)
  }

  @Test func evaluateRealCase() throws {
    // 이 맥의 python 3.9.6(시스템): 2025-10에 EOL 지남, 최신 사이클은 3.15
    let py = [c("3.15", "3.15.2", .date("2031-10-31")), c("3.9", "3.9.25", .date("2025-10-31"))]
    let r = try #require(EOL.evaluate(version: "3.9.6", cycles: py, now: now))
    #expect(r.cycle == "3.9" && r.status == .passed && r.update == .major && r.latestCycle == "3.15" && r.latestVersion == "3.9.25")
  }

  @Test func suggest() {
    #expect(EOL.suggestCommand(runtime: "node", manager: .brew, latestCycle: "26", hasMise: false) == "brew upgrade node")
    #expect(EOL.suggestCommand(runtime: "java", manager: .brew, latestCycle: "25", hasMise: false) == "brew upgrade openjdk")
    #expect(EOL.suggestCommand(runtime: "python", manager: .pyenv, latestCycle: "3.15", hasMise: false) == "pyenv install 3.15 && pyenv global 3.15")
    #expect(EOL.suggestCommand(runtime: "python", manager: .system, latestCycle: "3.15", hasMise: true)?.hasPrefix("mise use -g python@3.15") == true)
    #expect(EOL.suggestCommand(runtime: "python", manager: .system, latestCycle: "3.15", hasMise: false)?.hasPrefix("brew install python") == true)
    #expect(EOL.suggestCommand(runtime: "go", manager: .manual, latestCycle: "1.27", hasMise: false) == nil)
  }

  @Test func cacheUsedThenRefreshedThenStaleFallback() async throws {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("macinv-eol-\(UUID().uuidString)")
    let url = dir.appendingPathComponent("eol-cache.json")
    defer { try? FileManager.default.removeItem(at: dir) }
    let body = Data(#"[{"cycle":"26","eol":"2029-04-30","latest":"26.13.1"}]"#.utf8)
    let calls = Counter()
    // 1) 캐시 없음 → 조회
    var got = await EOL.cycles(for: ["node", "swift"], cacheURL: url, now: now) { _ in calls.inc(); return body }
    #expect(got["node"]?.first?.cycle == "26" && got["swift"] == nil && calls.n == 1)   // swift는 대상 아님
    // 2) 7일 안 → 캐시만
    got = await EOL.cycles(for: ["node"], cacheURL: url, now: now.addingTimeInterval(86400)) { _ in calls.inc(); return nil }
    #expect(got["node"] != nil && calls.n == 1)
    // 3) 7일 지남 + 오프라인 → 오래된 캐시라도
    got = await EOL.cycles(for: ["node"], cacheURL: url, now: now.addingTimeInterval(8 * 86400)) { _ in calls.inc(); return nil }
    #expect(got["node"]?.first?.latest == "26.13.1" && calls.n == 2)
  }
}

final class Counter: @unchecked Sendable {
  private let lock = NSLock(); private var v = 0
  func inc() { lock.lock(); v += 1; lock.unlock() }
  var n: Int { lock.lock(); defer { lock.unlock() }; return v }
}

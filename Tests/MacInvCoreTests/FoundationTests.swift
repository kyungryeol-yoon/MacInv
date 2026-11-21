import Testing
import Foundation
@testable import MacInvCore

@Suite("SearchPath")
struct SearchPathTests {
  @Test func mergeKeepsOrderAndDedups() {
    let got = SearchPath.merge(["/a", "/b/", "", "/a"], ["/c", "/b", " /d "])
    #expect(got == ["/a", "/b", "/c", "/d"])
  }

  @Test func shellPathWinsOverKnownDirs() {
    // 로그인 셸 PATH의 순서(nvm의 node가 brew node보다 앞)가 그대로 유지돼야 한다
    let shell = ["/Users/me/.nvm/versions/node/v22/bin", "/opt/homebrew/bin"]
    let got = SearchPath.merge(shell, SearchPath.knownDirs(home: "/Users/me"))
    #expect(got.first == "/Users/me/.nvm/versions/node/v22/bin")
    #expect(got.filter { $0 == "/opt/homebrew/bin" }.count == 1)
  }

  @Test func parseMarkedIgnoresShellNoise() {
    let out = "Last login: ...\nwelcome!\n\(SearchPath.beginMarker)/x/bin:/usr/bin\(SearchPath.endMarker)\nbye"
    #expect(SearchPath.parseMarked(out) == ["/x/bin", "/usr/bin"])
  }

  @Test func parseMarkedMissingMarkers() {
    #expect(SearchPath.parseMarked("/usr/bin:/bin") == nil)
    #expect(SearchPath.parseMarked("\(SearchPath.beginMarker)\(SearchPath.endMarker)") == nil)
  }

  @Test func findReturnsFirstExecutable() {
    let exists: Set<String> = ["/b/brew", "/c/brew"]
    #expect(SearchPath.find("brew", in: ["/a", "/b", "/c"]) { exists.contains($0) } == "/b/brew")
    #expect(SearchPath.find("npm", in: ["/a", "/b"]) { exists.contains($0) } == nil)
  }
}

@Suite("CommandRunner")
struct CommandRunnerTests {
  let runner = CommandRunner(searchPath: ["/bin", "/usr/bin"])

  @Test func runsAndCapturesOutput() async throws {
    let r = try await runner.run("echo", ["안녕", "macinv"])
    #expect(r.stdout == "안녕 macinv\n")
    #expect(r.exitCode == 0)
  }

  @Test func missingToolIsNotFound() async {
    await #expect(throws: CommandError.notFound("definitely-not-a-tool")) {
      try await runner.run("definitely-not-a-tool")
    }
  }

  @Test func readModeAcceptsNonzeroExitWithStdout() async throws {
    // npm ls처럼 정상인데 exit 1 — 읽기에서는 stdout이 있으면 성공
    let r = try await runner.run("sh", ["-c", "echo data; exit 1"])
    #expect(r.stdout == "data\n")
    #expect(r.exitCode == 1)
  }

  @Test func strictModeFailsOnNonzeroExitAndCarriesStderr() async {
    await #expect(throws: CommandError.failed(command: "sh -c echo data; echo boom >&2; exit 2", exitCode: 2, detail: "boom")) {
      try await runner.run("sh", ["-c", "echo data; echo boom >&2; exit 2"], strict: true)
    }
  }

  @Test func readModeFailsWhenNoStdout() async {
    await #expect(throws: CommandError.self) {
      try await runner.run("sh", ["-c", "exit 3"])
    }
  }

  @Test func timeoutStopsTheProcess() async {
    let start = Date()
    await #expect(throws: CommandError.timedOut(command: "sleep 10", seconds: 0.5)) {
      try await runner.run("sleep", ["10"], timeout: 0.5)
    }
    #expect(Date().timeIntervalSince(start) < 5)
  }

  @Test func largeOutputDoesNotDeadlock() async throws {
    // 파이프 버퍼(64KB)를 넘는 출력도 끝까지 읽는다
    let r = try await runner.run("sh", ["-c", "yes x | head -n 200000"])
    #expect(r.stdout.count == 400_000)
  }

  @Test func manyConcurrentRunsDoNotHang() async throws {
    // 회귀: 동시 실행이 스레드 풀보다 많으면 블로킹 읽기·waitUntilExit 때문에 멈췄다
    let start = Date()
    let codes = try await withThrowingTaskGroup(of: Int32.self) { g in
      for i in 0..<40 { g.addTask { try await runner.run("sh", ["-c", "sleep 0.2; echo \(i); echo err >&2"]).exitCode } }
      return try await g.reduce(into: [Int32]()) { $0.append($1) }
    }
    #expect(codes.count == 40 && codes.allSatisfy { $0 == 0 })
    #expect(Date().timeIntervalSince(start) < 20)
  }

  @Test func exitCodeAfterSuspension() async throws {
    // run() 뒤 await로 다른 스레드에서 이어져도 종료 코드를 받는다
    let r = try await runner.run("sh", ["-c", "sleep 0.3; echo x; exit 4"], okCodes: [4])
    #expect(r.exitCode == 4 && r.stdout == "x\n")
  }

  @Test func childGetsSearchPath() async throws {
    let r = try await CommandRunner(searchPath: ["/opt/x/bin", "/bin", "/usr/bin"]).run("sh", ["-c", "printf %s \"$PATH\""])
    #expect(r.stdout == "/opt/x/bin:/bin:/usr/bin")
  }
}

@Suite("Permissions")
struct PermissionsTests {
  @Test func decide() {
    #expect(Permissions.decide([nil, true, false]) == .granted)
    #expect(Permissions.decide([false, nil]) == .denied)
    #expect(Permissions.decide([nil, nil]) == .unknown)
    #expect(Permissions.decide([]) == .unknown)
  }

  @Test func notice() {
    #expect(Permissions.notice(.granted, grantedBefore: false) == .none)
    #expect(Permissions.notice(.granted, grantedBefore: true) == .none)
    #expect(Permissions.notice(.unknown, grantedBefore: true) == .none)
    #expect(Permissions.notice(.denied, grantedBefore: false) == .notGranted)
    #expect(Permissions.notice(.denied, grantedBefore: true) == .lostAfterUpdate)   // 업데이트·다시 빌드 후 풀림
  }
}

@Suite("SelfIdentity")
struct SelfIdentityTests {
  let me = SelfIdentity(bundlePath: "/Users/me/dev/MacInv/build/MacInv.app", home: "/Users/me")

  @Test(arguments: [
    "/Applications/MacInv.app",
    "/Applications/MacInv.app/Contents/MacOS/MacInv",
    "/Users/me/dev/MacInv/build/MacInv.app",
    "/Users/me/Library/Application Support/MacInv",
    "/Users/me/Library/Application Support/MacInv/cache.json",
    "/Users/me/Library/Preferences/io.github.kyungryeol-yoon.macinv.plist",
    "/Users/me/Library/Caches/io.github.kyungryeol-yoon.macinv",
    "/Users/me/Library/HTTPStorages/io.github.kyungryeol-yoon.macinv",
    "/Users/me/Library/Saved Application State/io.github.kyungryeol-yoon.macinv.savedState",
  ])
  func selfPaths(_ path: String) { #expect(me.isSelf(path: path)) }

  @Test(arguments: [
    "/Applications/MacInvaders.app",                                          // 이름만 비슷한 다른 앱
    "/Applications/MacInv.app.backup",
    "/Users/me/Library/Application Support/MacInventory",
    "/Users/me/Library/Preferences/io.github.kyungryeol-yoon.macinvx.plist",  // 접두사만 같은 다른 ID
    "/Users/me/Library/Preferences/io.github.kyungryeol-yoon.hum.plist",
    "/Users/me/Library/Caches/com.example.macinv",
  ])
  func notSelfPaths(_ path: String) { #expect(!me.isSelf(path: path)) }

  @Test func caskAndBundleID() {
    #expect(me.isSelf(caskToken: "macinv"))
    #expect(me.isSelf(caskToken: "MacInv"))
    #expect(!me.isSelf(caskToken: "diswitch"))
    #expect(me.isSelf(bundleID: "io.github.kyungryeol-yoon.macinv"))
    #expect(!me.isSelf(bundleID: "io.github.kyungryeol-yoon.hum"))
  }
}

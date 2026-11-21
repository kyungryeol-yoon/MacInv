import Testing
import Foundation
@testable import MacInvCore

@Suite("Disk")
struct DiskTests {
  let home = "/Users/me"

  // MARK: 위치 표 불변식(데이터를 고칠 때 깨지면 안 되는 것)

  @Test func tableLoads() {
    #expect(Disk.locations.count == 18)
    #expect(Set(Disk.locations.map(\.id)).count == Disk.locations.count)   // id 중복 없음
  }

  @Test func noDeadEnds() {
    // 네이티브 정리 명령도 휴지통도 없으면 note로 이유를 남겨야 한다(사용자를 막다른 길에 두지 않기)
    for l in Disk.locations where (l.cleanup ?? []).isEmpty && l.trash != true {
      #expect(!(l.note ?? "").isEmpty, "\(l.id): 정리 방법도 note도 없음")
    }
  }

  @Test func electronNeverTargetsUserData() {
    // 로그인 세션·로컬 데이터 — 지우면 캐시 정리가 아니라 데이터 삭제. 편의를 위해 추가하지 말 것
    let forbidden = ["IndexedDB", "Local Storage", "Cookies", "Session Storage"]
    for l in Disk.locations { for g in l.globs { for f in forbidden { #expect(!g.contains(f), "\(l.id): \(g)") } } }
  }

  @Test func neverIsNeverCleanable() {
    #expect(!Tier.never.cleanable(includeExpensive: true))
    #expect(!Tier.expensive.cleanable(includeExpensive: false))
    #expect(Tier.expensive.cleanable(includeExpensive: true))
    #expect(Tier.safe.cleanable(includeExpensive: false) && Tier.artifact.cleanable(includeExpensive: false))
  }

  // MARK: 경로

  @Test func expandWildcards() {
    let fs = fakeFS([
      "\(home)/Library/Application Support": ["Notion", "Slack", "Code"],
      "\(home)/Library/Application Support/Notion": ["Cache", "Partitions", "IndexedDB"],
      "\(home)/Library/Application Support/Notion/Partitions": ["ws1", "ws2"],
      "\(home)/Library/Application Support/Notion/Partitions/ws1": ["Cache"],
      "\(home)/Library/Application Support/Slack": ["Code Cache"],
      "\(home)/Library/Group Containers": ["HUAQ24HBR6.dev.orbstack", "X.dev.orbstackish"],
    ])
    #expect(Disk.expand("~/Library/Application Support/*/Cache", home: home, fs: fs) == ["\(home)/Library/Application Support/Notion/Cache"])
    #expect(Disk.expand("~/Library/Application Support/*/Partitions/*/Cache", home: home, fs: fs) == ["\(home)/Library/Application Support/Notion/Partitions/ws1/Cache"])
    #expect(Disk.expand("~/Library/Group Containers/*.dev.orbstack", home: home, fs: fs) == ["\(home)/Library/Group Containers/HUAQ24HBR6.dev.orbstack"])
    #expect(Disk.expand("~/nothing/*", home: home, fs: fs).isEmpty)
  }

  @Test func resolveSkipsAppleAndNests() {
    let fs = fakeFS([
      "\(home)/Library/Application Support": ["com.apple.Safari", "Claude"],
      "\(home)/Library/Application Support/com.apple.Safari": ["Cache"],
      "\(home)/Library/Application Support/Claude": ["Cache", "Code Cache"],
    ])
    let got = Disk.resolve(home: home, fs: fs)
    #expect(got["electron-cache"]?.sorted() == ["\(home)/Library/Application Support/Claude/Cache", "\(home)/Library/Application Support/Claude/Code Cache"])
    #expect(Disk.hasAppleComponent("\(home)/Library/Application Support/com.apple.Safari/Cache"))
  }

  @Test func electronPathsNeverNest() {
    // 와일드카드끼리 같은 경로를 두 번 세지 않는다(부모·자식이면 부모 하나)
    let all = Disk.locations.first { $0.id == "electron-cache" }!.globs
    for a in all { for b in all where a != b { #expect(!b.hasPrefix(a + "/"), "\(a) ⊃ \(b)") } }
  }

  @Test func breakdownAggregatesPartitionsPerApp() {
    let s = ["\(home)/Library/Application Support/Notion/Cache": 100,
             "\(home)/Library/Application Support/Notion/Partitions/ws1/Cache": 50,
             "\(home)/Library/Application Support/Claude/Code Cache": 200]
    let got = Disk.groupUsage(home: home, groupBy: Disk.groupByAppSupportDir, sizes: s)
    #expect(got.map(\.name) == ["Claude", "Notion"])
    #expect(got.map(\.kb) == [200, 150])
    #expect(Disk.groupUsage(home: home, groupBy: nil, sizes: s).isEmpty)
  }

  // MARK: 고아 캐시

  func loc(_ id: String) -> DiskLocation { Disk.locations.first { $0.id == id }! }

  @Test func orphanBasics() {
    #expect(Disk.isOrphan(loc("cargo-registry"), installed: []))                 // cargo도 RustRover도 없음
    #expect(!Disk.isOrphan(loc("cargo-registry"), installed: ["cargo"]))
    #expect(!Disk.isOrphan(loc("electron-cache"), installed: []))                // 도구가 정해지지 않은 위치는 판정 안 함
  }

  @Test func ownerAppsPreventFalseOrphan() {
    // ③ gradle CLI 없이 Android Studio만 써도 ~/.gradle을 쓴다
    var inv = Inventory()
    inv.apps = [AppItem(name: "Android Studio", path: "/Applications/Android Studio.app", bundleID: "com.google.android.studio", version: nil, dateAdded: nil)]
    let tools = Disk.installedTools(inv)
    #expect(tools.contains("android studio") && tools.contains("androidstudio"))
    #expect(!Disk.isOrphan(loc("gradle-cache"), installed: tools))
  }

  @Test func pathCheckPreventsFalseOrphan() {
    // ② 인벤토리에 런타임이 없어도 PATH에 pip3가 있으면 pip 캐시는 고아가 아니다
    let tools = Disk.detectTools(Inventory()) { $0 == "pip3" }
    #expect(!Disk.isOrphan(loc("pip-cache"), installed: tools))
    #expect(Disk.isOrphan(loc("pip-cache"), installed: Disk.detectTools(Inventory()) { _ in false }))
  }

  @Test func aliasesFromInventory() {
    var inv = Inventory()
    inv.runtimes = [Runtime(name: "python", installed: true, version: "3.12", origins: [])]
    inv.apps = [AppItem(name: "Xcode", path: "/Applications/Xcode.app", bundleID: "com.apple.dt.Xcode", version: nil, dateAdded: nil)]
    let tools = Disk.installedTools(inv)
    #expect(tools.contains("pip") && tools.contains("simctl") && tools.contains("xcodebuild"))
    #expect(!Disk.isOrphan(loc("pip-cache"), installed: tools))
    #expect(!Disk.isOrphan(loc("xcode-deriveddata"), installed: tools))
  }
}

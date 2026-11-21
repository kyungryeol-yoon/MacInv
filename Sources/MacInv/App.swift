import SwiftUI
import MacInvCore

@main
struct MacInvApp: App {
  @StateObject private var model = AppModel()

  var body: some Scene {
    MenuBarExtra {
      MenuPanel().environmentObject(model)
    } label: {
      // 메뉴 막대 아이콘(2026-11-21 정함) checklist — 업데이트가 있으면 주황 점
      Image(nsImage: MenuBarIcon.image(badge: model.pendingUpdates > 0))
    }
    .menuBarExtraStyle(.window)   // 메뉴가 아니라 카드 패널

    Window("MacInv", id: "main") {
      MainWindow().environmentObject(model)
    }
    .defaultSize(width: 980, height: 640)
    .defaultLaunchBehavior(.suppressed)   // 켤 때 창은 안 띄움
  }
}

/// 앱 전체 상태: 명령을 찾을 PATH, 권한, 인벤토리.
@MainActor
final class AppModel: ObservableObject {
  /// 환경 탭에 보여 줄 도구
  static let tools = ["brew", "npm", "node", "pipx", "cargo", "go", "mas", "python3"]
  /// 전체 디스크 접근이 한 번이라도 허용된 적 있는지 — 풀렸을 때 "업데이트 후 풀림"으로 안내하려고
  private static let grantedOnceKey = "fullDiskAccessGrantedOnce"

  @Published var runner = CommandRunner(searchPath: SearchPath.knownDirs(home: NSHomeDirectory()))
  @Published var pathFromShell = false
  @Published var found: [String: String] = [:]
  @Published var fullDiskAccess: Permissions.Access = .unknown
  @Published var inventory: Inventory?
  /// 업데이트 대기(nil = 아직 확인 전)
  @Published var outdated: [OutdatedItem]?
  @Published var outdatedWarnings: [String] = []
  @Published var checkingUpdates = false
  /// 런타임 이름 → 지원 종료·최신 버전(조회 실패면 비어 있음 — 에러가 아니다)
  @Published var runtimeEOL: [String: RuntimeEOL] = [:]
  @Published var hasMise = false
  /// 디스크 사용처(캐시·모델·빌드 산출물). 인벤토리가 있어야 고아 캐시를 판정한다
  @Published var disk: DiskReport?
  @Published var scanningDisk = false
  /// 메뉴에서 누른 항목으로 창의 분류를 바꾸라는 요청
  @Published var requestedSection: MainWindow.Section?
  @Published var scanning = false
  @Published var measuring = false
  /// 데이터 용량 측정 진행(끝낸 앱 수, 전체)
  @Published var measureProgress: (done: Int, total: Int)?
  private var observers: [NSObjectProtocol] = []

  var measuringText: String? {
    guard measuring else { return nil }
    if let p = measureProgress { return "용량 재는 중 \(p.done)/\(p.total)" }
    return "용량 재는 중…"
  }

  /// 실제로 할 수 있는 업데이트 수(고정된 것 제외)
  var pendingUpdates: Int { outdated?.filter { !$0.pinned }.count ?? 0 }

  func open(_ section: MainWindow.Section, using openWindow: OpenWindowAction) {
    requestedSection = section
    openWindow(id: "main")
    NSApplication.shared.activate()
  }

  var notice: Permissions.Notice {
    Permissions.notice(fullDiskAccess, grantedBefore: UserDefaults.standard.bool(forKey: Self.grantedOnceKey))
  }

  init() {
    checkPermissions()
    scan()
    // 패널·창이 앞으로 올 때마다 권한을 다시 본다(설정에서 켜고 돌아오면 바로 반영 — 단 실제 적용은 다시 시작 후)
    observers.append(NotificationCenter.default.addObserver(forName: NSWindow.didBecomeKeyNotification, object: nil, queue: .main) { [weak self] _ in
      MainActor.assumeIsolated { self?.checkPermissions() }
    })
  }

  func checkPermissions() {
    fullDiskAccess = Permissions.fullDiskAccess()
    if fullDiskAccess == .granted { UserDefaults.standard.set(true, forKey: Self.grantedOnceKey) }
  }

  /// PATH 확인 → 인벤토리 수집 → (목록을 먼저 보여 주고) 앱 번들 용량 채우기.
  func scan() {
    guard !scanning else { return }
    scanning = true
    checkPermissions()
    Task {
      let (dirs, fromShell) = await SearchPath.resolve()
      let r = CommandRunner(searchPath: dirs)
      var f: [String: String] = [:]
      for t in Self.tools { if let p = r.resolve(t) { f[t] = p } }
      runner = r; pathFromShell = fromShell; found = f
      // 업데이트 대기는 따로(npm outdated는 네트워크를 탄다) — 목록·용량과 나란히
      checkingUpdates = true
      Task {
        let u = await Updates.scan(runner: r)
        outdated = u.items; outdatedWarnings = u.warnings; checkingUpdates = false
      }
      let inv = await InventoryScanner.scan(runner: r)
      inventory = inv; scanning = false
      hasMise = r.resolve("mise") != nil
      scanningDisk = true
      Task { disk = await Disk.scan(runner: r, inventory: inv); scanningDisk = false }
      // 런타임 EOL: 7일 캐시, 실패·오프라인이면 조용히 생략
      Task {
        let installed = inv.runtimes.filter(\.installed)
        let cycles = await EOL.cycles(for: installed.map(\.name))
        var m: [String: RuntimeEOL] = [:]
        for rt in installed { if let c = cycles[rt.name], let e = EOL.evaluate(version: rt.version, cycles: c) { m[rt.name] = e } }
        runtimeEOL = m
      }
      // 번들(du 한 번) → 데이터(앱마다, 앱 하나 끝날 때마다 진행률). 목록은 먼저 보여 준다.
      measuring = true
      let sized = await InventoryScanner.measureBundles(inv.apps, runner: r)
      inventory?.apps = sized
      measureProgress = (0, sized.count)
      let withData = await InventoryScanner.measureData(sized, runner: r) { done, total in
        Task { @MainActor in self.measureProgress = (done, total) }
      }
      inventory?.apps = withData; measuring = false; measureProgress = nil
    }
  }

  /// 권한은 다시 시작해야 적용된다 — 1초 뒤 같은 번들을 다시 열고 지금 것은 끈다.
  func relaunch() {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: "/bin/sh")
    p.arguments = ["-c", "sleep 1; /usr/bin/open \"$0\"", Bundle.main.bundlePath]
    try? p.run()
    NSApplication.shared.terminate(nil)
  }
}

/// 메뉴 막대 아이콘: checklist(템플릿). 알릴 것이 있으면 주황 점을 얹는다.
/// 점이 있으면 템플릿으로 둘 수 없으므로(점까지 흑백이 됨) 기호는 labelColor로 직접 칠한다 — 그리는 순간의 막대 모양(밝음/어두움)을 따른다.
enum MenuBarIcon {
  static func image(badge: Bool) -> NSImage {
    let config = NSImage.SymbolConfiguration(pointSize: 14, weight: .regular)
    guard let symbol = NSImage(systemSymbolName: "checklist", accessibilityDescription: "MacInv")?.withSymbolConfiguration(config) else { return NSImage() }
    guard badge else { symbol.isTemplate = true; return symbol }
    let size = NSSize(width: symbol.size.width + 3, height: symbol.size.height)
    let img = NSImage(size: size, flipped: false) { _ in
      let tinted = symbol.withSymbolConfiguration(config.applying(.init(paletteColors: [.labelColor]))) ?? symbol
      tinted.draw(in: NSRect(x: 0, y: 0, width: symbol.size.width, height: symbol.size.height))
      let d: CGFloat = 6
      NSColor.systemOrange.setFill()
      NSBezierPath(ovalIn: NSRect(x: size.width - d, y: size.height - d, width: d, height: d)).fill()
      return true
    }
    img.isTemplate = false
    img.accessibilityDescription = "MacInv — 업데이트 있음"
    return img
  }
}

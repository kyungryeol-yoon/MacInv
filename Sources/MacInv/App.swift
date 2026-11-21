import SwiftUI
import Combine
import MacInvCore

/// 메뉴 막대 아이콘은 AppKit `NSStatusItem`으로 둔다(Hum과 같은 방식) — SwiftUI `MenuBarExtra`는 오른쪽 클릭을 받지 못한다.
/// 왼쪽 클릭: 카드 패널(팝오버) · 오른쪽 클릭: 짧은 메뉴(열기·다시 스캔·끝내기). 무심코 오른쪽 클릭하는 습관을 받아 주려고(2026-11-21 사용자 요청).
@main
struct MacInvApp: App {
  @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
  var body: some Scene { Settings { EmptyView() } }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, NSPopoverDelegate, NSWindowDelegate {
  private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
  private let popover = NSPopover()
  private var model: AppModel!
  private var window: NSWindow?
  private var outsideClick: Any?
  private var bag: Set<AnyCancellable> = []

  func applicationDidFinishLaunching(_ note: Notification) {
    model = AppModel()
    model.openMain = { [weak self] in self?.showMain() }
    if let b = item.button {
      b.target = self
      b.action = #selector(clicked(_:))
      b.sendAction(on: [.leftMouseUp, .rightMouseUp])
      b.toolTip = "MacInv"
    }
    let panel = NSHostingController(rootView: MenuPanel().environmentObject(model))
    panel.sizingOptions = .preferredContentSize
    popover.contentViewController = panel
    popover.behavior = .transient
    popover.delegate = self
    // 업데이트가 있으면 주황 점 — @Published는 바뀌기 직전에 알려 주므로 다음 차례에 읽는다
    updateIcon()
    model.objectWillChange.sink { [weak self] in DispatchQueue.main.async { self?.updateIcon() } }.store(in: &bag)
  }

  private var badge: Bool?
  private func updateIcon() {
    let b = model.pendingUpdates > 0
    guard b != badge else { return }
    badge = b
    item.button?.image = MenuBarIcon.image(badge: b)
  }

  // MARK: 누르기 — 왼쪽: 패널 / 오른쪽: 짧은 메뉴
  @objc private func clicked(_ sender: NSStatusBarButton) {
    if NSApp.currentEvent?.type == .rightMouseUp {
      popover.performClose(nil)
      let m = NSMenu()
      m.addItem(withTitle: "MacInv 열기", action: #selector(openAction), keyEquivalent: "o").target = self
      let scan = m.addItem(withTitle: model.scanning ? "스캔 중…" : "다시 스캔", action: model.scanning ? nil : #selector(scanAction), keyEquivalent: "r")
      scan.target = self
      m.addItem(.separator())
      m.addItem(withTitle: "MacInv 끝내기", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
      item.menu = m
      sender.performClick(nil)
      item.menu = nil
      return
    }
    if popover.isShown {
      popover.performClose(nil)
    } else {
      model.checkPermissions()   // 설정에서 켜고 돌아왔으면 바로 반영
      NSApp.activate()
      popover.show(relativeTo: sender.bounds, of: sender, preferredEdge: .minY)
      // .transient만으로는 다른 앱 창을 눌러도 안 닫힐 때가 있다(Hum 실측) → 바깥 클릭을 직접 감시
      outsideClick = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown, .otherMouseDown]) { [weak self] _ in
        MainActor.assumeIsolated { self?.popover.performClose(nil) }
      }
    }
  }

  func popoverDidClose(_ notification: Notification) {
    if let m = outsideClick { NSEvent.removeMonitor(m); outsideClick = nil }
  }

  @objc private func openAction() { showMain() }
  @objc private func scanAction() { model.scan(refresh: true) }

  // MARK: 목록 창
  /// 창은 처음 열 때 만들고, 닫아도 버리지 않는다(다시 열면 보던 곳 그대로). 켤 때는 띄우지 않는다.
  private func showMain() {
    popover.performClose(nil)
    if window == nil {
      let host = NSHostingController(rootView: MainWindow().environmentObject(model))
      host.sceneBridgingOptions = .all   // 툴바·검색창·제목을 SwiftUI에서 창으로
      let w = NSWindow(contentViewController: host)
      w.title = "MacInv"
      w.styleMask = [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView]
      w.isReleasedWhenClosed = false
      w.setContentSize(NSSize(width: 980, height: 640))
      w.setFrameAutosaveName("MacInvMain")
      if !w.setFrameUsingName("MacInvMain") { w.center() }
      w.delegate = self
      window = w
    }
    NSApp.activate()
    window?.makeKeyAndOrderFront(nil)
  }
}

struct ShownPlan: Identifiable {
  enum Phase: Equatable { case review, running, done(ExecSummary) }
  let id = UUID()
  var plan: Plan
  var measuring: Bool
  var phase = Phase.review
  var lines: [ExecLine] = []
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
  /// 지금 보고 있는 정리 계획(읽기 전용 — 실행은 6단계)
  @Published var shownPlan: ShownPlan?
  private var observers: [NSObjectProtocol] = []

  var measuringText: String? {
    guard measuring else { return nil }
    if let p = measureProgress { return "용량 재는 중 \(p.done)/\(p.total)" }
    return "용량 재는 중…"
  }

  /// 실제로 할 수 있는 업데이트 수(고정된 것 제외)
  var pendingUpdates: Int { outdated?.filter { !$0.pinned }.count ?? 0 }

  /// 목록 창을 띄우는 쪽(AppDelegate)이 넣어 준다
  var openMain: () -> Void = {}

  func open(_ section: MainWindow.Section) {
    requestedSection = section
    openMain()
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
  /// - refresh: 업데이트를 보기 전에 `brew update`로 brew의 버전 정보를 새로 받는다(「다시 스캔」 버튼). 켤 때 자동 스캔은 받지 않는다.
  ///   `brew outdated`는 가지고 있는 정보로만 비교해서, 받지 않으면 새 버전이 안 보인다(2026-11-21 실측 — 직접 만든 tap의 Hum 0.1.1이
  ///   `brew update` 전까지 안 보임). 설치된 것은 바꾸지 않는다.
  func scan(refresh: Bool = false) {
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
        var refreshWarning: String?
        if refresh, r.resolve("brew") != nil {
          do { _ = try await r.run("brew", ["update", "--quiet"], timeout: 180, strict: true) }
          catch { refreshWarning = "brew 정보를 새로 받지 못했습니다(인터넷 연결 확인) — 마지막으로 받은 정보로 보여 줍니다." }
        }
        let u = await Updates.scan(runner: r)
        outdated = u.items; outdatedWarnings = u.warnings + [refreshWarning].compactMap { $0 }; checkingUpdates = false
      }
      let inv = await InventoryScanner.scan(runner: r)
      inventory = inv; scanning = false
      hasMise = r.resolve("mise") != nil
      scanningDisk = true
      let running = Running.current()
      Task { disk = await Disk.scan(runner: r, inventory: inv, running: running); scanningDisk = false }
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

  /// 계획을 만들어 보여 주고, 담긴 경로를 잰다(아무것도 지우지 않는다).
  func showPlan(_ make: (Inventory, Planner.Env) -> Plan) {
    guard let inv = inventory else { return }
    let plan = make(inv, Planner.Env(home: NSHomeDirectory(), me: SelfIdentity.current(), running: Running.current()))
    let shown = ShownPlan(plan: plan, measuring: plan.hasWork)
    shownPlan = shown
    guard plan.hasWork else { return }
    let r = runner
    Task {
      let measured = await Planner.measure(plan, runner: r)
      if shownPlan?.id == shown.id { shownPlan?.plan = measured; shownPlan?.measuring = false }
    }
  }

  /// 확인 화면에서 승인한 계획을 실행한다(한 번에 하나씩, 줄 단위 로그). 끝나도 시트는 열어 두고 결과를 보여 준다.
  func execute(low: Set<String>, skip: Set<String> = []) {
    guard let shown = shownPlan, shown.phase == .review, shown.plan.hasWork else { return }
    shownPlan?.phase = .running
    let ex = Executor(run: Executor.commands(runner), runningOwner: { path in await MainActor.run { Self.runningOwner(of: path) } },
                      me: SelfIdentity.current())
    Task {
      let sum = await ex.execute(shown.plan.without(skip), low: low) { line in
        await MainActor.run {
          guard self.shownPlan?.id == shown.id else { return }
          if let i = self.shownPlan?.lines.firstIndex(where: { $0.id == line.id }) { self.shownPlan?.lines[i] = line } else { self.shownPlan?.lines.append(line) }
        }
      }
      if shownPlan?.id == shown.id { shownPlan?.phase = .done(sum) }
    }
  }

  /// 확인 화면 닫기. 실행했으면 바뀐 것을 반영하려고 다시 스캔한다.
  func closePlan() {
    let ran = shownPlan.map { if case .done = $0.phase { return true } else { return false } } ?? false
    shownPlan = nil
    if ran { scan() }
  }

  /// 이 경로를 쓰고 있는 실행 중 앱: 앱 번들이면 그 번들이 떠 있는지, 캐시면 Application Support/<앱> 폴더 이름으로.
  /// 조회 실패면 안전 쪽(실행 중으로 본다).
  static func runningOwner(of path: String) -> String? {
    if path.hasSuffix(".app") {
      return NSWorkspace.shared.runningApplications.first { app in
        guard let b = app.bundleURL?.path else { return false }
        return b == path || b.hasPrefix(path + "/")
      }.map { $0.localizedName ?? (path as NSString).lastPathComponent }
    }
    guard let dir = Disk.appSupportDirName(home: NSHomeDirectory(), path: path) else { return nil }
    guard let running = Running.current() else { return "실행 중인 앱 확인 실패" }
    return Running.matches(dirName: dir, running: running) ? dir : nil
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

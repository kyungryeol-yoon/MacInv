import SwiftUI
import MacInvCore

@main
struct MacInvApp: App {
  @StateObject private var model = AppModel()

  var body: some Scene {
    MenuBarExtra {
      MenuContent().environmentObject(model)
    } label: {
      // 메뉴 막대 아이콘(2026-11-21 정함) checklist — 업데이트가 있으면 주황 점
      Image(nsImage: MenuBarIcon.image(badge: model.pendingUpdates > 0))
    }

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
    // 메뉴를 열 때마다 권한을 다시 본다(설정에서 켜고 돌아오면 바로 반영 — 단 실제 적용은 다시 시작 후)
    observers.append(NotificationCenter.default.addObserver(forName: NSMenu.didBeginTrackingNotification, object: nil, queue: .main) { [weak self] _ in
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

struct MenuContent: View {
  @EnvironmentObject private var model: AppModel
  @Environment(\.openWindow) private var openWindow

  var body: some View {
    if let inv = model.inventory {
      Text("앱 \(inv.apps.count)개 · brew \(inv.formulae.count + inv.casks.count)개 · 패키지 \(inv.packages.count)개")
      Text("스캔 \(inv.scannedAt.formatted(date: .omitted, time: .shortened))\(model.measuringText.map { " · " + $0 } ?? "")")
        .foregroundStyle(.secondary)
      if !inv.warnings.isEmpty { Text("⚠️ 경고 \(inv.warnings.count)건 — 창에서 확인").foregroundStyle(.secondary) }
    } else {
      Text("스캔 중…")
    }
    if let o = model.outdated {
      let n = model.pendingUpdates, majors = o.filter { $0.runtimeMajor && !$0.pinned }.count
      Button(n == 0 ? "업데이트 없음" : "업데이트 \(n)개" + (majors > 0 ? " (런타임 메이저 \(majors)개)" : "") + "…") {
        model.open(.updates, using: openWindow)
      }
      .disabled(n == 0)
    } else if model.checkingUpdates {
      Text("업데이트 확인 중…").foregroundStyle(.secondary)
    }
    Divider()
    Button("MacInv 열기") { model.open(.apps, using: openWindow) }
      .keyboardShortcut("o")
    Button(model.scanning ? "스캔 중…" : "다시 스캔") { model.scan() }
      .disabled(model.scanning)

    if model.fullDiskAccess == .denied {
      Divider()
      if model.notice == .lostAfterUpdate {
        Text("전체 디스크 접근이 풀렸습니다(업데이트 후 생길 수 있음)")
        Text("시스템 설정 목록에서 MacInv를 −로 지운 뒤 다시 추가하세요").foregroundStyle(.secondary)
      } else {
        Text("전체 디스크 접근: 꺼져 있음 — 다른 앱 데이터는 일부만 측정")
      }
      Button("시스템 설정 열기…") { NSWorkspace.shared.open(Permissions.fullDiskAccessSettingsURL) }
      Text("켠 뒤에는 MacInv를 다시 시작해야 적용됩니다").foregroundStyle(.secondary)
      Button("MacInv 다시 시작") { model.relaunch() }
    }
    Divider()
    Button("MacInv 종료") { NSApplication.shared.terminate(nil) }
      .keyboardShortcut("q")
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

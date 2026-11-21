import SwiftUI
import MacInvCore

@main
struct MacInvApp: App {
  @StateObject private var env = Environment()

  var body: some Scene {
    MenuBarExtra {
      MenuContent().environmentObject(env)
    } label: {
      Image(systemName: "checklist")   // 메뉴 막대 아이콘(2026-11-21 정함) — 앱 아이콘의 "목록 + 줄 끝 상태"와 같은 모양
    }
  }
}

/// 앱이 돌아가는 환경: 명령을 찾을 PATH, 찾은 도구, 권한.
@MainActor
final class Environment: ObservableObject {
  /// 인벤토리에 쓰는 도구(1단계 확인용 — 소스가 생기면 각 소스가 자기 도구를 찾는다)
  static let tools = ["brew", "npm", "node", "pipx", "cargo", "go", "mas", "python3"]
  /// 전체 디스크 접근이 한 번이라도 허용된 적 있는지 — 풀렸을 때 "업데이트 후 풀림"으로 안내하려고
  private static let grantedOnceKey = "fullDiskAccessGrantedOnce"

  @Published var runner = CommandRunner(searchPath: SearchPath.knownDirs(home: NSHomeDirectory()))
  @Published var pathFromShell = false
  @Published var found: [String: String] = [:]
  @Published var fullDiskAccess: Permissions.Access = .unknown
  @Published var loading = true
  private var observers: [NSObjectProtocol] = []

  var notice: Permissions.Notice {
    Permissions.notice(fullDiskAccess, grantedBefore: UserDefaults.standard.bool(forKey: Self.grantedOnceKey))
  }

  init() {
    refresh()
    // 메뉴를 열 때마다 권한을 다시 본다(설정에서 켜고 돌아오면 바로 반영 — 단 실제 적용은 다시 시작 후)
    observers.append(NotificationCenter.default.addObserver(forName: NSMenu.didBeginTrackingNotification, object: nil, queue: .main) { [weak self] _ in
      MainActor.assumeIsolated { self?.checkPermissions() }
    })
  }

  func checkPermissions() {
    fullDiskAccess = Permissions.fullDiskAccess()
    if fullDiskAccess == .granted { UserDefaults.standard.set(true, forKey: Self.grantedOnceKey) }
  }

  func refresh() {
    loading = true
    checkPermissions()
    Task {
      let (dirs, fromShell) = await SearchPath.resolve()
      let r = CommandRunner(searchPath: dirs)
      var f: [String: String] = [:]
      for t in Self.tools { if let p = r.resolve(t) { f[t] = p } }
      runner = r; pathFromShell = fromShell; found = f; loading = false
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
  @EnvironmentObject private var env: Environment

  var body: some View {
    Section("권한") {
      switch env.fullDiskAccess {
      case .granted: Text("전체 디스크 접근: 허용됨")
      case .unknown: Text("전체 디스크 접근: 확인 못 함")
      case .denied:
        if env.notice == .lostAfterUpdate {
          Text("전체 디스크 접근이 풀렸습니다(업데이트 후 생길 수 있음)")
          Text("시스템 설정 목록에서 MacInv를 −로 지운 뒤 다시 추가하세요").foregroundStyle(.secondary)
        } else {
          Text("전체 디스크 접근: 꺼져 있음 — 다른 앱 데이터는 일부만 측정")
        }
        Button("시스템 설정 열기…") { NSWorkspace.shared.open(Permissions.fullDiskAccessSettingsURL) }
        Text("켠 뒤에는 MacInv를 다시 시작해야 적용됩니다").foregroundStyle(.secondary)
        Button("MacInv 다시 시작") { env.relaunch() }
      }
    }
    Section(env.loading ? "도구 찾는 중…" : (env.pathFromShell ? "도구 (로그인 셸 PATH)" : "도구 (기본 경로만 — 셸 PATH를 못 읽음)")) {
      ForEach(Environment.tools, id: \.self) { t in
        if let p = env.found[t] { Text("\(t)  \(p)") } else if !env.loading { Text("\(t)  없음").foregroundStyle(.secondary) }
      }
    }
    Divider()
    Button("다시 확인") { env.refresh() }
    Button("MacInv 종료") { NSApplication.shared.terminate(nil) }
      .keyboardShortcut("q")
  }
}

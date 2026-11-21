import SwiftUI

@main
struct MacInvApp: App {
  var body: some Scene {
    MenuBarExtra {
      MenuContent()
    } label: {
      Image(systemName: "checklist")   // 메뉴 막대 아이콘(2026-11-21 정함) — 앱 아이콘의 "목록 + 줄 끝 상태"와 같은 모양
    }
  }
}

struct MenuContent: View {
  var body: some View {
    Text("MacInv — 만드는 중")
    Divider()
    Button("MacInv 종료") { NSApplication.shared.terminate(nil) }
      .keyboardShortcut("q")
  }
}

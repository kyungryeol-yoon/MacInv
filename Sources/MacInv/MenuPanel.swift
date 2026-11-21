import SwiftUI
import MacInvCore

/// 메뉴 막대 아이콘을 누르면 뜨는 패널 — 맨 위 "열기", 읽기 전용 요약은 카드(누르면 창이 그 목록으로 열림).
/// 카드 모양은 같은 개발자의 Hum과 맞춘다(둥근 10, 옅은 바탕, 얇은 테두리, 색 아이콘 칸).
struct MenuPanel: View {
  @EnvironmentObject private var model: AppModel
  @Environment(\.openWindow) private var openWindow

  var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      header
      VStack(spacing: 6) {
        updatesCard
        diskCard
        installedCard
        runtimeCard
        permissionCard
      }
      footer
    }
    .padding(16)
    .frame(width: 340)
  }

  // MARK: 맨 위

  private var header: some View {
    HStack(spacing: 10) {
      Image(nsImage: NSApplication.shared.applicationIconImage).resizable().frame(width: 40, height: 40)
      VStack(alignment: .leading, spacing: 2) {
        Text("MacInv").font(.title3.weight(.semibold))
        Text(subtitle).font(.caption).foregroundStyle(.secondary).lineLimit(1)
      }
      Spacer()
      Button { model.open(.apps, using: openWindow) } label: { Text("열기").frame(minWidth: 44) }
        .buttonStyle(.borderedProminent)
        .keyboardShortcut("o")
    }
  }

  private var subtitle: String {
    if model.scanning { return "스캔 중…" }
    guard let inv = model.inventory else { return "" }
    var s = "스캔 \(inv.scannedAt.formatted(date: .omitted, time: .shortened))"
    if let m = model.measuringText { s += " · " + m }
    return s
  }

  // MARK: 카드

  @ViewBuilder private var updatesCard: some View {
    if let o = model.outdated {
      let n = model.pendingUpdates
      let majors = o.filter { $0.runtimeMajor && !$0.pinned }.count
      PanelCard(symbol: "arrow.up.circle.fill", color: n > 0 ? .blue : .green, title: "업데이트",
                value: n == 0 ? "최신" : "\(n)개",
                details: n == 0 ? ["brew · npm · App Store 모두 최신"] : [sourceLine(o), majors > 0 ? "⚠️ 런타임 메이저 \(majors)개 — 따로 확인" : nil].compactMap { $0 },
                action: n > 0 ? { model.open(.updates, using: openWindow) } : nil)
    } else {
      PanelCard(symbol: "arrow.up.circle.fill", color: .secondary, title: "업데이트", value: "", details: ["확인 중…"], action: nil)
    }
  }

  private func sourceLine(_ o: [OutdatedItem]) -> String {
    let live = o.filter { !$0.pinned }
    let parts: [(String, Int)] = [("brew", live.filter { $0.source == .brewFormula }.count), ("cask", live.filter { $0.source == .brewCask }.count),
                                  ("npm", live.filter { $0.source == .npm }.count), ("App Store", live.filter { $0.source == .mas }.count)]
    return parts.filter { $0.1 > 0 }.map { "\($0.0) \($0.1)" }.joined(separator: " · ")
  }

  @ViewBuilder private var diskCard: some View {
    if let d = model.disk {
      let orphans = d.entries.filter(\.orphan).count
      var details = ["재다운로드 비싼 것 \(sizeText(d.expensiveKB))" + (d.freeBytes.map { " · 디스크 여유 \(ByteCountFormatter.string(fromByteCount: $0, countStyle: .file))" } ?? "")]
      if let top = d.entries.first { details.append("가장 큰 곳: \(top.location.name) \(sizeText(top.sizeKB))") }
      let blocked = d.blockedByApp
      if !blocked.isEmpty { details.append("⏸ 실행 중이라 뺌: " + blocked.prefix(2).map { "\($0.name) \(sizeText($0.kb))" }.joined(separator: " · ")) }
      if orphans > 0 { details.append("⚠️ 고아 캐시 \(orphans)개") }
      return PanelCard(symbol: "internaldrive.fill", color: orphans > 0 ? .orange : .green, title: "정리 가능",
                       value: sizeText(d.reclaimableKB), details: details) { model.open(.disk, using: openWindow) }
    } else {
      return PanelCard(symbol: "internaldrive.fill", color: .secondary, title: "디스크", value: "", details: ["재는 중…"], action: nil)
    }
  }

  @ViewBuilder private var installedCard: some View {
    if let inv = model.inventory {
      PanelCard(symbol: "square.grid.2x2.fill", color: .purple, title: "설치된 것", value: "앱 \(inv.apps.count)개",
                details: ["brew \(inv.formulae.count + inv.casks.count) · 패키지 \(inv.packages.count) · 런타임 \(inv.runtimes.filter(\.installed).count) · 백그라운드 \(inv.background.count)"]) {
        model.open(.apps, using: openWindow)
      }
    }
  }

  @ViewBuilder private var runtimeCard: some View {
    let ended = model.runtimeEOL.filter { $0.value.status == .passed }.keys.sorted()
    if !ended.isEmpty {
      PanelCard(symbol: "clock.badge.exclamationmark.fill", color: .red, title: "지원 끝난 런타임", value: "\(ended.count)개",
                details: [ended.joined(separator: " · ")]) { model.open(.runtimes, using: openWindow) }
    }
  }

  @ViewBuilder private var permissionCard: some View {
    if model.fullDiskAccess == .denied {
      VStack(alignment: .leading, spacing: 8) {
        HStack(spacing: 10) {
          PanelBadge(symbol: "lock.shield.fill", color: .orange)
          VStack(alignment: .leading, spacing: 2) {
            Text(model.notice == .lostAfterUpdate ? "전체 디스크 접근이 풀렸습니다" : "전체 디스크 접근 꺼짐").font(.callout.weight(.semibold))
            Text(model.notice == .lostAfterUpdate ? "업데이트 후 생길 수 있어요 — 목록에서 MacInv를 지운 뒤 다시 추가하세요"
                                                   : "목록은 그대로 보입니다. 다른 앱의 보호 폴더 용량만 일부만 잽니다")
              .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
          }
        }
        HStack {
          Button("시스템 설정 열기…") { NSWorkspace.shared.open(Permissions.fullDiskAccessSettingsURL) }
          Button("다시 시작") { model.relaunch() }.help("켠 뒤에는 MacInv를 다시 시작해야 적용됩니다")
        }
        .controlSize(.small)
      }
      .cardBackground()
    }
  }

  // MARK: 아래

  private var footer: some View {
    HStack {
      Button { model.scan() } label: { Label(model.scanning ? "스캔 중…" : "다시 스캔", systemImage: "arrow.clockwise") }
        .disabled(model.scanning)
      Spacer()
      Button { NSApplication.shared.terminate(nil) } label: { Label("MacInv 끝내기", systemImage: "power") }
        .keyboardShortcut("q")
    }
    .buttonStyle(.borderless)
    .font(.callout)
    .foregroundStyle(.secondary)
  }
}

/// 카드 한 장: 색 아이콘 · 제목 · 오른쪽 값 · 설명 줄. action이 있으면 누를 수 있고(올리면 살짝 밝아짐) 오른쪽에 ›.
struct PanelCard: View {
  let symbol: String
  let color: Color
  let title: String
  let value: String
  let details: [String]
  let action: (() -> Void)?
  @State private var hover = false

  var body: some View {
    let content = HStack(alignment: .top, spacing: 10) {
      PanelBadge(symbol: symbol, color: color)
      VStack(alignment: .leading, spacing: 3) {
        HStack {
          Text(title).font(.callout.weight(.semibold))
          Spacer()
          Text(value).font(.callout.weight(.semibold)).monospacedDigit()
          if action != nil { Image(systemName: "chevron.right").font(.caption.weight(.semibold)).foregroundStyle(.tertiary) }
        }
        ForEach(details, id: \.self) { Text($0).font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true) }
      }
    }
    .cardBackground(highlight: hover && action != nil)
    .contentShape(Rectangle())

    if let action {
      Button(action: action) { content }.buttonStyle(.plain).onHover { hover = $0 }
    } else {
      content
    }
  }
}

/// 항목 아이콘 — 항목 색을 옅게 깐 둥근 네모(Hum과 같은 모양)
struct PanelBadge: View {
  let symbol: String
  let color: Color
  var body: some View {
    Image(systemName: symbol)
      .font(.system(size: 12, weight: .semibold))
      .foregroundStyle(color)
      .frame(width: 26, height: 26)
      .background(color.opacity(0.18), in: RoundedRectangle(cornerRadius: 7))
  }
}

extension View {
  /// 카드 바탕 — 살짝 띄우고 얇은 테두리(Hum과 같은 값)
  func cardBackground(highlight: Bool = false) -> some View {
    self
      .padding(.horizontal, 12).padding(.vertical, 10)
      .frame(maxWidth: .infinity, alignment: .leading)
      .background(Color.primary.opacity(highlight ? 0.10 : 0.06), in: RoundedRectangle(cornerRadius: 10))
      .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(Color.primary.opacity(0.08)))
  }
}

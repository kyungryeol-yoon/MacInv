import SwiftUI
import MacInvCore

/// 실행 전 확인 화면 → 실행 로그 → 결과. 무엇이 어디로 가는지(휴지통·네이티브 명령), 무엇을 왜 남기는지를 먼저 보여 주고,
/// 명시적으로 누를 때만 실행한다(엔터·더블클릭에 묶지 않음). [확신낮음]은 기본 해제. 되돌리기 어려운 것은 한 번 더 묻는다.
struct PlanSheet: View {
  let shown: ShownPlan
  @EnvironmentObject private var model: AppModel
  @State private var low: Set<String> = []
  @State private var confirming = false
  /// 사용자가 뺀 대상 — 대상이 여럿일 때 하나씩 뺄 수 있다(고아 잔여물에서 모르는 이름 등)
  @State private var skipped: Set<String> = []
  private var plan: Plan { shown.plan.without(skipped) }
  private var full: Plan { shown.plan }

  var body: some View {
    VStack(spacing: 0) {
      HStack(spacing: 10) {
        Image(systemName: plan.verb == "업데이트" ? "arrow.up.circle" : "trash").font(.title2).foregroundStyle(.secondary)
        VStack(alignment: .leading, spacing: 2) {
          Text(plan.title).font(.title3.weight(.semibold))
          Text(subtitle).font(.caption).foregroundStyle(.secondary)
        }
        Spacer()
      }
      .padding(16)
      Divider()
      if shown.phase != .review {
        ExecLog(lines: shown.lines)
      } else {
      ScrollView {
        VStack(alignment: .leading, spacing: 14) {
          ForEach(plan.refusals, id: \.self) { Callout(symbol: "lock.fill", color: .secondary, text: $0) }
          ForEach(plan.warnings, id: \.self) { Callout(symbol: "exclamationmark.triangle.fill", color: .orange, text: $0) }
          ForEach(full.targets) { t in
            TargetSection(target: t, low: $low, measuring: shown.measuring,
                          included: full.targets.count > 1 ? Binding(get: { !skipped.contains(t.id) },
                                                                     set: { if $0 { skipped.remove(t.id) } else { skipped.insert(t.id) } }) : nil)
          }
          if !plan.held.isEmpty {
            Group2(title: "이번에 빼 둔 것 \(plan.held.count)개 — 필요하면 직접 하세요") {
              ForEach(plan.held) { h in
                VStack(alignment: .leading, spacing: 2) {
                  HStack(spacing: 6) { Image(systemName: "pause.circle").foregroundStyle(.secondary); Text(h.name).font(.callout.weight(.semibold)) }
                  Text(h.reason).font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                  if let c = h.command { Text(shellLine(c)).font(.system(.caption, design: .monospaced)).textSelection(.enabled) }
                }
              }
            }
          }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
      }
      }
      Divider()
      footer.padding(16)
    }
    .frame(minWidth: 640, idealWidth: 720, minHeight: 420, idealHeight: 560)
  }

  private var subtitle: String {
    switch shown.phase {
    case .review: return "실행하기 전에 무엇을 하는지 확인하세요 — 아직 아무것도 \(plan.verb == "업데이트" ? "바꾸지" : "지우지") 않았습니다"
    case .running: return "진행 중… 끝날 때까지 이 창을 닫지 않습니다"
    case .done: return "끝났습니다 — 닫으면 목록을 다시 스캔합니다"
    }
  }

  @ViewBuilder private var footer: some View {
    switch shown.phase {
    case .review: reviewFooter
    case .running:
      HStack { ProgressView().controlSize(.small); Text("\(shown.lines.filter { $0.state != .running }.count)개 끝남").foregroundStyle(.secondary); Spacer() }
    case .done(let s):
      HStack(alignment: .center, spacing: 12) {
        VStack(alignment: .leading, spacing: 2) {
          Text("성공 \(s.ok)" + (s.failed > 0 ? " · 실패 \(s.failed)" : "") + (s.skipped > 0 ? " · 건너뜀 \(s.skipped)" : ""))
            .foregroundStyle(s.failed > 0 ? .orange : .primary)
          if s.trashed > 0 { Text("휴지통으로 \(s.trashed)개 옮겼습니다 — 휴지통을 비워야 공간이 실제로 늘어납니다. 비우기 전엔 되돌릴 수 있습니다.").font(.caption).foregroundStyle(.secondary) }
          if s.selfUpdated { Text("MacInv가 업데이트됐습니다 — 다시 시작해야 새 버전이 됩니다.").font(.caption).foregroundStyle(.secondary) }
        }
        Spacer()
        if s.selfUpdated { Button("다시 시작") { model.relaunch() } }
        Button("닫기") { model.closePlan() }.keyboardShortcut(.cancelAction)
      }
    }
  }

  private var reviewFooter: some View {
    HStack(alignment: .center, spacing: 12) {
      VStack(alignment: .leading, spacing: 2) {
        if plan.hasWork {
          if shown.measuring {
            HStack(spacing: 6) { ProgressView().controlSize(.small); Text("용량 재는 중…").foregroundStyle(.secondary) }
          } else if let kb = plan.reclaimKB(includeLow: low) {
            let all = plan.reclaimKB(includeLow: Set(plan.lowPaths)) ?? kb
            Text("💾 회수 예상 \(sizeText(kb))" + (all > kb ? "  (확신 낮음 모두 포함 시 \(sizeText(all)))" : "")).monospacedDigit()
          }
          if plan.targets.contains(where: { !$0.trash.isEmpty }) {
            Text("휴지통으로 옮깁니다 — 휴지통을 비우기 전까지는 디스크가 실제로 비지 않습니다.").font(.caption).foregroundStyle(.secondary)
          }
        }
      }
      Spacer()
      Button("취소") { model.closePlan() }.keyboardShortcut(.cancelAction)
      Button(plan.verb) { if plan.requireConfirm { confirming = true } else { model.execute(low: low, skip: skipped) } }
        .buttonStyle(.borderedProminent)
        .tint(plan.verb == "업데이트" ? .accentColor : .red)
        .disabled(!plan.hasWork)
    }
    .confirmationDialog(confirmTitle, isPresented: $confirming) {
      Button(plan.verb, role: plan.verb == "업데이트" ? nil : .destructive) { model.execute(low: low, skip: skipped) }
      Button("취소", role: .cancel) {}
    } message: { Text(confirmMessage) }
  }

  private var confirmTitle: String {
    plan.targets.contains { $0.kind == .ollama } ? "모델을 지울까요?" : plan.targets.contains { $0.kind == .orphan } ? "고아 잔여물을 옮길까요?"
      : plan.targets.contains { $0.kind == .disk } ? "재다운로드가 비싼 것도 정리할까요?" : "\(plan.title)할까요?"
  }

  private var confirmMessage: String {
    if plan.targets.contains(where: { $0.kind == .ollama }) { return "ollama rm은 휴지통을 거치지 않습니다 — 다시 쓰려면 모델을 다시 받아야 합니다." }
    if plan.targets.contains(where: { $0.kind == .orphan }) { return "설치된 앱이 없는 것으로 보이는 \(plan.targets.count)개의 파일을 휴지통으로 옮깁니다. 휴지통을 비우기 전엔 되돌릴 수 있습니다." }
    if plan.targets.contains(where: { $0.kind == .disk }) { return "다시 받는 데 오래 걸리거나 용량이 큰 것이 들어 있습니다." }
    return "앱을 지우면 설정·데이터도 함께 휴지통으로 갑니다. brew가 지우는 것은 휴지통을 거치지 않습니다."
  }
}

/// 실행 로그 — 한 줄에 하나, 진행 중·성공·건너뜀·실패와 이유
private struct ExecLog: View {
  let lines: [ExecLine]
  var body: some View {
    ScrollViewReader { proxy in
      List(lines) { l in
        HStack(alignment: .firstTextBaseline, spacing: 8) {
          Group {
            switch l.state {
            case .running: ProgressView().controlSize(.mini)
            case .ok: Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
            case .skipped: Image(systemName: "pause.circle.fill").foregroundStyle(.secondary)
            case .failed: Image(systemName: "xmark.circle.fill").foregroundStyle(.red)
            }
          }
          .frame(width: 16)
          VStack(alignment: .leading, spacing: 1) {
            Text(l.text.replacingOccurrences(of: NSHomeDirectory(), with: "~")).font(.system(.callout, design: .monospaced)).lineLimit(2).truncationMode(.middle).textSelection(.enabled)
            if let d = l.detail { Text(d).font(.caption).foregroundStyle(l.state == .failed ? .red : .secondary).fixedSize(horizontal: false, vertical: true).textSelection(.enabled) }
          }
        }
        .id(l.id)
      }
      .onChange(of: lines.count) { _, _ in if let last = lines.last { proxy.scrollTo(last.id, anchor: .bottom) } }
    }
  }
}

private struct TargetSection: View {
  let target: PlanTarget
  @Binding var low: Set<String>
  let measuring: Bool
  /// 대상이 여럿이면 체크박스로 넣고 뺀다
  var included: Binding<Bool>? = nil

  var body: some View {
    VStack(alignment: .leading, spacing: 8) {
      HStack(spacing: 6) {
        Text(target.kind.label).font(.caption.weight(.semibold)).padding(.horizontal, 6).padding(.vertical, 2)
          .background(Color.secondary.opacity(0.15), in: Capsule())
        if let included { Toggle("", isOn: included).toggleStyle(.checkbox).labelsHidden() }
        Text(target.name).font(.headline).foregroundStyle(included?.wrappedValue == false ? .secondary : .primary)
        if let id = target.bundleID { Text(id).font(.caption).foregroundStyle(.secondary) }
      }
      ForEach(target.warnings, id: \.self) { Callout(symbol: "exclamationmark.triangle.fill", color: .orange, text: $0) }

      if !target.commands.isEmpty {
        Group2(title: "실행할 명령") {
          ForEach(Array(target.commands.enumerated()), id: \.offset) { _, c in
            Text(shellLine(c)).font(.system(.callout, design: .monospaced)).textSelection(.enabled)
          }
        }
      }
      if !target.trash.isEmpty {
        Group2(title: "휴지통으로 옮길 것 \(target.trash.count)개") {
          ForEach(target.trash) { PathRow(p: $0, low: $low, measuring: measuring) }
        }
      }
      if !target.needsAdmin.isEmpty {
        Group2(title: "🔑 권한이 필요해 자동으로 옮기지 않는 것 \(target.needsAdmin.count)개") {
          ForEach(target.needsAdmin) { PathRow(p: $0, low: nil, measuring: measuring) }
        }
      }
      if !target.needsPermission.isEmpty {
        Group2(title: "🛡 macOS가 막고 있어 지금은 못 옮기는 것 \(target.needsPermission.count)개") {
          ForEach(target.needsPermission) { PathRow(p: $0, low: nil, measuring: measuring) }
          PermissionHint(paths: target.needsPermission.map(\.path))
        }
      }
      if !target.excluded.isEmpty {
        Group2(title: "남기는 것 \(target.excluded.count)개") {
          ForEach(target.excluded) { e in
            HStack(alignment: .firstTextBaseline, spacing: 8) {
              Image(systemName: "lock.fill").font(.caption).foregroundStyle(.secondary).frame(width: 16)
              VStack(alignment: .leading, spacing: 1) {
                Text(tilde(e.path)).lineLimit(1).truncationMode(.middle)
                Text(e.reason).font(.caption).foregroundStyle(.secondary)
              }
            }
          }
        }
      }
      ForEach(target.suggestions, id: \.self) { s in
        Label(s, systemImage: "info.circle").font(.callout).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
      }
    }
  }
}

private struct PathRow: View {
  let p: PlanPath
  /// nil이면 고를 수 없는 줄(권한 필요)
  let low: Binding<Set<String>>?
  let measuring: Bool

  var body: some View {
    HStack(alignment: .firstTextBaseline, spacing: 8) {
      if p.confidence == .low, let low {
        Toggle("", isOn: Binding(get: { low.wrappedValue.contains(p.path) },
                                 set: { if $0 { low.wrappedValue.insert(p.path) } else { low.wrappedValue.remove(p.path) } }))
          .toggleStyle(.checkbox).labelsHidden().frame(width: 16)
          .help("확신 낮음 — 이름으로 찾은 것이라 기본으로 빼 둡니다. 이 앱 것이 맞으면 체크하세요")
      } else {
        Image(systemName: low == nil ? "nosign" : "checkmark").font(.caption).foregroundStyle(.secondary).frame(width: 16)
      }
      ConfidenceBadge(c: p.confidence)
      VStack(alignment: .leading, spacing: 1) {
        Text(tilde(p.path)).lineLimit(1).truncationMode(.middle)
        Text(p.reason).font(.caption).foregroundStyle(.secondary)
      }
      Spacer()
      Text(p.sizeKB.map { sizeText($0) } ?? (measuring ? "…" : "—")).monospacedDigit().foregroundStyle(.secondary)
    }
    .contextMenu { Button("Finder에서 보기") { NSWorkspace.shared.selectFile(p.path, inFileViewerRootedAtPath: "") } }
  }
}

/// macOS 보호로 막힌 경로: 다른 앱의 .app은 「앱 관리」, 데이터 폴더는 「전체 디스크 접근」.
/// 권한은 켠 뒤 MacInv를 다시 시작해야 적용된다(1단계 실측).
private struct PermissionHint: View {
  let paths: [String]
  @EnvironmentObject private var model: AppModel
  static let appManagementURL = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AppBundles")!
  var body: some View {
    let bundles = paths.contains { $0.hasSuffix(".app") }, data = paths.contains { !$0.hasSuffix(".app") }
    VStack(alignment: .leading, spacing: 6) {
      Text([bundles ? "다른 개발자의 앱 번들은 「앱 관리」 권한이 있어야 옮길 수 있습니다." : nil,
            data ? "다른 앱의 데이터 폴더는 「전체 디스크 접근」 권한이 있어야 옮길 수 있습니다." : nil].compactMap { $0 }.joined(separator: " ")
           + " 관리자 암호로는 풀리지 않습니다 — 시스템 설정에서 MacInv를 켠 뒤 다시 시작하세요.")
        .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
      HStack {
        if bundles { Button("앱 관리 설정 열기…") { NSWorkspace.shared.open(Self.appManagementURL) } }
        if data { Button("전체 디스크 접근 설정 열기…") { NSWorkspace.shared.open(Permissions.fullDiskAccessSettingsURL) } }
        Button("다시 시작") { model.relaunch() }
      }
      .controlSize(.small)
    }
    .padding(.top, 4)
  }
}

private struct ConfidenceBadge: View {
  let c: Confidence
  var body: some View {
    let (t, color): (String, Color) = switch c { case .high: ("확실", .green); case .medium: ("중간", .blue); case .low: ("낮음", .orange) }
    Text(t).font(.caption2.weight(.semibold)).foregroundStyle(color)
      .padding(.horizontal, 5).padding(.vertical, 1)
      .background(color.opacity(0.15), in: RoundedRectangle(cornerRadius: 4))
      .help(c == .high ? "Bundle ID가 정확히 맞음" : c == .medium ? "벤더 폴더 전체 등 — 이 앱 것으로 보이지만 추론" : "이름으로 찾음 — 다른 것일 수 있음")
  }
}

private struct Group2<Content: View>: View {
  let title: String
  @ViewBuilder let content: Content
  var body: some View {
    VStack(alignment: .leading, spacing: 6) {
      Text(title).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
      VStack(alignment: .leading, spacing: 6) { content }
        .padding(10).frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.primary.opacity(0.04), in: RoundedRectangle(cornerRadius: 8))
    }
  }
}

private struct Callout: View {
  let symbol: String
  let color: Color
  let text: String
  var body: some View {
    Label { Text(text).fixedSize(horizontal: false, vertical: true) } icon: { Image(systemName: symbol).foregroundStyle(color) }
      .padding(10).frame(maxWidth: .infinity, alignment: .leading)
      .background(color.opacity(0.10), in: RoundedRectangle(cornerRadius: 8))
  }
}

func tilde(_ path: String) -> String {
  let home = NSHomeDirectory()
  return path.hasPrefix(home + "/") ? "~" + path.dropFirst(home.count) : path
}

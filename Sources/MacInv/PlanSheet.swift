import SwiftUI
import MacInvCore

/// 정리 계획 보기 — **읽기 전용.** 무엇이 어디로 가는지(휴지통·네이티브 명령), 무엇을 왜 남기는지를 보여 준다.
/// 실행 버튼은 6단계에서 연결한다. [확신낮음]은 기본 해제(직접 골라야 포함).
struct PlanSheet: View {
  let shown: ShownPlan
  @Environment(\.dismiss) private var dismiss
  @State private var low: Set<String> = []
  private var plan: Plan { shown.plan }

  var body: some View {
    VStack(spacing: 0) {
      HStack(spacing: 10) {
        Image(systemName: plan.verb == "업데이트" ? "arrow.up.circle" : "trash").font(.title2).foregroundStyle(.secondary)
        VStack(alignment: .leading, spacing: 2) {
          Text(plan.title).font(.title3.weight(.semibold))
          Text("실행하기 전에 무엇을 하는지 확인하세요 — 아직 아무것도 \(plan.verb == "업데이트" ? "바꾸지" : "지우지") 않았습니다").font(.caption).foregroundStyle(.secondary)
        }
        Spacer()
      }
      .padding(16)
      Divider()
      ScrollView {
        VStack(alignment: .leading, spacing: 14) {
          ForEach(plan.refusals, id: \.self) { Callout(symbol: "lock.fill", color: .secondary, text: $0) }
          ForEach(plan.warnings, id: \.self) { Callout(symbol: "exclamationmark.triangle.fill", color: .orange, text: $0) }
          ForEach(plan.targets) { TargetSection(target: $0, low: $low, measuring: shown.measuring) }
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
      Divider()
      footer.padding(16)
    }
    .frame(minWidth: 640, idealWidth: 720, minHeight: 420, idealHeight: 560)
  }

  private var footer: some View {
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
      Button("닫기") { dismiss() }.keyboardShortcut(.cancelAction)
      Button(plan.verb) {}.disabled(true).help("실행은 다음 단계(6단계)에서 연결합니다")
    }
  }
}

private struct TargetSection: View {
  let target: PlanTarget
  @Binding var low: Set<String>
  let measuring: Bool

  var body: some View {
    VStack(alignment: .leading, spacing: 8) {
      HStack(spacing: 6) {
        Text(target.kind.label).font(.caption.weight(.semibold)).padding(.horizontal, 6).padding(.vertical, 2)
          .background(Color.secondary.opacity(0.15), in: Capsule())
        Text(target.name).font(.headline)
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

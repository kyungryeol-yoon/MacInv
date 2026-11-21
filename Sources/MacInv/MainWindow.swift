import SwiftUI
import MacInvCore

/// 목록 창: 왼쪽 분류, 오른쪽 표.
struct MainWindow: View {
  @EnvironmentObject private var model: AppModel
  @State private var section: Section? = .apps
  @State private var query = ""

  enum Section: String, Hashable, CaseIterable {
    case apps, formulae, casks, packages, runtimes, background, environment
    var title: String {
      switch self {
      case .apps: return "앱"
      case .formulae: return "Homebrew 포뮬러"
      case .casks: return "Homebrew 캐스크"
      case .packages: return "패키지"
      case .runtimes: return "런타임"
      case .background: return "백그라운드 항목"
      case .environment: return "환경"
      }
    }
    var symbol: String {
      switch self {
      case .apps: return "square.grid.2x2"
      case .formulae: return "terminal"
      case .casks: return "shippingbox"
      case .packages: return "cube"
      case .runtimes: return "chevron.left.forwardslash.chevron.right"
      case .background: return "gearshape.2"
      case .environment: return "gearshape"
      }
    }
  }

  var body: some View {
    NavigationSplitView {
      List(selection: $section) {
        ForEach(Section.allCases, id: \.self) { s in
          HStack {
            Label(s.title, systemImage: s.symbol)
            Spacer()
            if let n = count(s) { Text("\(n)").foregroundStyle(.secondary).monospacedDigit() }
          }
          .tag(s)
        }
      }
      .navigationSplitViewColumnWidth(min: 190, ideal: 210)
    } detail: {
      Group {
        if model.inventory == nil {
          ProgressView("스캔 중…").frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
          switch section ?? .apps {
          case .apps: AppsTable(apps: filtered(model.inventory!.apps) { $0.name + " " + ($0.bundleID ?? "") })
          case .formulae: FormulaeTable(items: filtered(model.inventory!.formulae) { $0.name })
          case .casks: CasksTable(items: filtered(model.inventory!.casks) { $0.token + " " + $0.appNames.joined(separator: " ") })
          case .packages: PackagesTable(items: filtered(model.inventory!.packages) { $0.name })
          case .runtimes: RuntimesView(runtimes: filtered(model.inventory!.runtimes) { $0.name }, managers: model.inventory!.versionManagers)
          case .background: BackgroundTable(items: filtered(model.inventory!.background) { ($0.label ?? "") + " " + $0.file + " " + $0.owner.label })
          case .environment: EnvironmentView()
          }
        }
      }
      .navigationTitle((section ?? .apps).title)
      .navigationSubtitle(subtitle)
      .searchable(text: $query, prompt: "이름으로 찾기")
      .toolbar {
        ToolbarItem {
          Button { model.scan() } label: { Label("다시 스캔", systemImage: "arrow.clockwise") }
            .disabled(model.scanning)
        }
      }
    }
  }

  private var subtitle: String {
    guard let inv = model.inventory else { return "" }
    var s = "스캔 \(inv.scannedAt.formatted(date: .omitted, time: .shortened))"
    if let m = model.measuringText { s += " · " + m }
    if model.fullDiskAccess == .denied { s += " · 전체 디스크 접근 꺼짐" }
    return s
  }

  private func count(_ s: Section) -> Int? {
    guard let inv = model.inventory else { return nil }
    switch s {
    case .apps: return inv.apps.count
    case .formulae: return inv.formulae.count
    case .casks: return inv.casks.count
    case .packages: return inv.packages.count
    case .runtimes: return inv.runtimes.filter(\.installed).count
    case .background: return inv.background.count
    case .environment: return inv.warnings.isEmpty ? nil : inv.warnings.count
    }
  }

  private func filtered<T>(_ items: [T], _ text: (T) -> String) -> [T] {
    let q = query.trimmingCharacters(in: .whitespaces)
    return q.isEmpty ? items : items.filter { text($0).localizedCaseInsensitiveContains(q) }
  }
}

// MARK: - 표

/// KB → "1.2 GB"
func sizeText(_ kb: Int?) -> String {
  guard let kb else { return "—" }
  let f = ByteCountFormatter()
  f.countStyle = .file
  f.allowsNonnumericFormatting = false   // "Zero KB" 대신 "0 KB"
  return f.string(fromByteCount: Int64(kb) * 1024)
}

/// 일부만 측정된 앱의 설명(마우스를 올리면 보임).
func partialHelp(_ a: AppItem) -> String {
  let home = NSHomeDirectory()
  var s = "⚠️ 일부만 측정 — 전체 디스크 접근 권한이 없어 이 앱의 데이터 폴더 일부를 읽지 못했습니다. 실제 용량은 이보다 큽니다."
  if !a.deniedPaths.isEmpty {
    s += "\n\n읽지 못한 곳:"
    for p in a.deniedPaths.prefix(6) { s += "\n• " + (p.hasPrefix(home) ? "~" + p.dropFirst(home.count) : p) }
    if a.deniedPaths.count > 6 { s += "\n• 외 \(a.deniedPaths.count - 6)곳" }
  }
  s += "\n\n정확히 재려면: 시스템 설정 → 개인정보 보호 및 보안 → 전체 디스크 접근 권한에서 MacInv를 켜고 MacInv를 다시 시작하세요."
  return s
}

struct AppsTable: View {
  let apps: [AppItem]
  @State private var order = [KeyPathComparator(\AppItem.name, comparator: .localizedStandard)]

  var body: some View {
    Table(apps.sorted(using: order), sortOrder: $order) {
      TableColumn("이름", value: \.name, comparator: .localizedStandard) { a in
        HStack(spacing: 8) {
          Image(nsImage: NSWorkspace.shared.icon(forFile: a.path)).resizable().frame(width: 18, height: 18)
          Text(a.name)
        }
      }
      .width(min: 160, ideal: 220)
      TableColumn("버전") { a in Text(a.version ?? "—").foregroundStyle(.secondary) }
        .width(min: 60, ideal: 90)
      TableColumn("출처", value: \.origin.label) { a in Text(a.origin.label) }
        .width(min: 90, ideal: 130)
      TableColumn("합계", value: \.totalKB.orZero) { a in
        HStack(spacing: 3) {
          Text(sizeText(a.totalKB)).monospacedDigit()
          if a.dataPartial { Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange).imageScale(.small) }
          Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .help(a.dataPartial ? partialHelp(a) : "번들 \(sizeText(a.bundleKB)) + 데이터 \(sizeText(a.dataKB))")
      }
      .width(min: 80, ideal: 95)
      TableColumn("번들", value: \.bundleKB.orZero) { a in Text(sizeText(a.bundleKB)).monospacedDigit().foregroundStyle(.secondary) }
        .width(min: 60, ideal: 75)
      TableColumn("데이터", value: \.dataKB.orZero) { a in
        HStack {
          Text(a.dataPartial ? "\(sizeText(a.dataKB)) 이상" : sizeText(a.dataKB)).monospacedDigit().foregroundStyle(.secondary)
          Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .help(a.dataPartial ? partialHelp(a) : "앱 번들 밖(~/Library 등)에 쌓인 데이터·캐시")
      }
      .width(min: 70, ideal: 95)
      TableColumn("위치") { a in Text((a.path as NSString).deletingLastPathComponent).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
        .width(min: 120, ideal: 200)
      TableColumn("번들 ID") { a in Text(a.bundleID ?? "—").foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
    }
  }
}

extension Optional where Wrapped == Int { var orZero: Int { self ?? -1 } }

struct FormulaeTable: View {
  let items: [BrewFormula]
  var body: some View {
    Table(items) {
      TableColumn("이름", value: \.name)
      TableColumn("버전") { f in Text(f.version ?? "—").foregroundStyle(.secondary) }
      TableColumn("설치") { f in Text(f.onRequest ? "직접 설치" : "의존성으로 설치").foregroundStyle(f.onRequest ? .primary : .secondary) }
      TableColumn("의존하는 것") { f in Text(f.dependencies.isEmpty ? "—" : f.dependencies.joined(separator: ", ")).foregroundStyle(.secondary).lineLimit(1) }
    }
  }
}

struct CasksTable: View {
  let items: [BrewCask]
  var body: some View {
    Table(items) {
      TableColumn("이름", value: \.token)
      TableColumn("버전") { c in Text(c.version ?? "—").foregroundStyle(.secondary) }
      TableColumn("설치한 앱") { c in Text(c.appNames.isEmpty ? "—" : c.appNames.joined(separator: ", ")) }
    }
  }
}

struct PackagesTable: View {
  let items: [PackageItem]
  var body: some View {
    if items.isEmpty {
      ContentUnavailableView("패키지 없음", systemImage: "cube", description: Text("npm 전역·pipx·cargo·Go·App Store(mas)로 설치한 것이 없거나, 그 도구가 없습니다."))
    } else {
      Table(items) {
        TableColumn("이름", value: \.name)
        TableColumn("버전") { p in Text(p.version ?? "—").foregroundStyle(.secondary) }
        TableColumn("관리자") { p in Text(p.manager.label) }
        TableColumn("경로") { p in Text(p.path ?? "—").foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
      }
    }
  }
}

struct EnvironmentView: View {
  @EnvironmentObject private var model: AppModel
  var body: some View {
    Form {
      SwiftUI.Section("권한") {
        LabeledContent("전체 디스크 접근") {
          switch model.fullDiskAccess {
          case .granted: Text("허용됨")
          case .denied: Text("꺼져 있음 — 다른 앱 데이터는 일부만 측정")
          case .unknown: Text("확인 못 함")
          }
        }
      }
      SwiftUI.Section(model.pathFromShell ? "도구 (로그인 셸 PATH)" : "도구 (기본 경로만 — 셸 PATH를 못 읽음)") {
        ForEach(AppModel.tools, id: \.self) { t in
          LabeledContent(t) { Text(model.found[t] ?? "없음").foregroundStyle(model.found[t] == nil ? .secondary : .primary) }
        }
      }
      if let w = model.inventory?.warnings, !w.isEmpty {
        SwiftUI.Section("경고") { ForEach(w, id: \.self) { Text($0) } }
      }
    }
    .formStyle(.grouped)
  }
}

struct RuntimesView: View {
  let runtimes: [Runtime]
  let managers: [VersionManagerInfo]
  var body: some View {
    VSplitView {
      Table(runtimes) {
        TableColumn("언어") { r in Text(r.name).foregroundStyle(r.installed ? .primary : .secondary) }
          .width(min: 70, ideal: 80)
        TableColumn("버전") { r in Text(r.installed ? (r.version ?? "?") : "설치 안 됨").foregroundStyle(r.installed ? .primary : .secondary) }
          .width(min: 70, ideal: 90)
        TableColumn("관리") { r in
          if let a = r.active { Text(a.manager.label + (a.package.map { " \($0)" } ?? "")) } else { Text("—").foregroundStyle(.secondary) }
        }
        .width(min: 80, ideal: 110)
        TableColumn("쓰이는 경로") { r in
          HStack(spacing: 4) {
            if r.conflict { Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange).help(conflictHelp(r)) }
            Text(r.active?.path ?? "—").foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
          }
        }
      }
      if !managers.isEmpty {
        Form {
          SwiftUI.Section("버전 매니저가 관리 중인 버전") {
            ForEach(managers) { m in LabeledContent(m.name) { Text(m.versions.joined(separator: ", ")).textSelection(.enabled) } }
          }
        }
        .formStyle(.grouped)
        .frame(minHeight: 120)
      }
    }
  }

  private func conflictHelp(_ r: Runtime) -> String {
    "PATH에 서로 다른 \(r.name)가 \(Set(r.origins.map(\.resolved)).count)개 있습니다 — 맨 위 것이 쓰입니다:\n" +
      r.origins.map { "\($0.path) (\($0.manager.label))" }.joined(separator: "\n")
  }
}

struct BackgroundTable: View {
  let items: [BackgroundItem]
  var body: some View {
    Table(items) {
      TableColumn("이름") { b in
        HStack(spacing: 4) {
          if b.owner == .unknown || b.programMissing { Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange) }
          Text(b.label ?? ((b.file as NSString).lastPathComponent as NSString).deletingPathExtension)
        }
      }
      .width(min: 200, ideal: 260)
      TableColumn("주인") { b in Text(b.owner.label).foregroundStyle(b.owner == .unknown ? .orange : .primary) }
        .width(min: 120, ideal: 170)
      TableColumn("상태") { b in Text(state(b)).foregroundStyle(b.programMissing || b.isEmpty ? .orange : .secondary) }
        .width(min: 90, ideal: 120)
      TableColumn("종류") { b in Text(b.kind.label).foregroundStyle(.secondary) }
        .width(min: 90, ideal: 110)
      TableColumn("실행 파일") { b in Text(b.program ?? "—").foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
    }
  }

  private func state(_ b: BackgroundItem) -> String {
    if b.isEmpty { return "내용 없음" }
    if b.programMissing { return "실행 파일 없음" }
    return b.runAtLoad ? "로그인 시 실행" : "필요할 때 실행"
  }
}

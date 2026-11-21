import SwiftUI
import MacInvCore

/// 목록 창: 왼쪽 분류, 오른쪽 표.
struct MainWindow: View {
  @EnvironmentObject private var model: AppModel
  @State private var section: Section? = .apps
  @State private var query = ""

  enum Section: String, Hashable, CaseIterable {
    case apps, formulae, casks, packages, environment
    var title: String {
      switch self {
      case .apps: return "앱"
      case .formulae: return "Homebrew 포뮬러"
      case .casks: return "Homebrew 캐스크"
      case .packages: return "패키지"
      case .environment: return "환경"
      }
    }
    var symbol: String {
      switch self {
      case .apps: return "square.grid.2x2"
      case .formulae: return "terminal"
      case .casks: return "shippingbox"
      case .packages: return "cube"
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
    if model.measuring { s += " · 용량 재는 중…" }
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
  return ByteCountFormatter.string(fromByteCount: Int64(kb) * 1024, countStyle: .file)
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
      TableColumn("번들 용량", value: \.bundleKB.orZero) { a in Text(sizeText(a.bundleKB)).monospacedDigit() }
        .width(min: 70, ideal: 90)
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

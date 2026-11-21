import SwiftUI
import MacInvCore

// 목록 표들 — 모든 열 제목을 눌러 정렬, 표 위 분류 버튼으로 나눠 보기,
// 건드리면 안 되는 것(애플 기본 앱·시스템 런타임·MacInv 자신·다른 것이 쓰는 포뮬러)은 🔒와 이유.

/// 표 위 분류 버튼(개수 포함).
struct FilterBar<F: Hashable>: View {
  let options: [(value: F, title: String, count: Int)]
  @Binding var selection: F
  var body: some View {
    HStack {
      Picker("보기", selection: $selection) {
        ForEach(options, id: \.value) { o in Text("\(o.title) \(o.count)").tag(o.value) }
      }
      .pickerStyle(.segmented)
      .labelsHidden()
      .fixedSize()
      Spacer()
    }
    .padding(.horizontal, 12).padding(.vertical, 8)
  }
}

/// 🔒 — 지우지 않는 것.
struct LockBadge: View {
  let text: String
  var body: some View { InfoBadge(systemImage: "lock.fill", color: .secondary, text: text) }
}

extension Optional where Wrapped == Int { var orZero: Int { self ?? -1 } }
extension Optional where Wrapped == String { var orDash: String { self ?? "—" } }

// MARK: 앱

struct AppsTable: View {
  let apps: [AppItem]
  enum Filter: Hashable { case all, other, appStore, brew, apple }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\AppItem.name, comparator: .localizedStandard)]
  private let me = SelfIdentity.current()

  private func kind(_ a: AppItem) -> Filter {
    switch a.origin { case .apple: return .apple; case .appStore: return .appStore; case .brewCask: return .brew; case .other: return .other }
  }

  var body: some View {
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", apps.count), (.other, "직접 설치", apps.filter { kind($0) == .other }.count),
                          (.appStore, "App Store", apps.filter { kind($0) == .appStore }.count),
                          (.brew, "Homebrew", apps.filter { kind($0) == .brew }.count),
                          (.apple, "Apple(보호)", apps.filter { kind($0) == .apple }.count)], selection: $filter)
      Table(apps.filter { filter == .all || kind($0) == filter }.sorted(using: order), sortOrder: $order) {
        TableColumn("이름", value: \.name, comparator: .localizedStandard) { a in
          HStack(spacing: 8) {
            Image(nsImage: NSWorkspace.shared.icon(forFile: a.path)).resizable().frame(width: 18, height: 18)
            Text(a.name)
            if a.origin == .apple { LockBadge(text: "애플 기본 앱입니다 — MacInv는 지우지 않습니다.\n\nmacOS에 딸린 앱이라 지우면 시스템 기능이 깨질 수 있습니다.") }
            if me.isSelf(path: a.path) { LockBadge(text: "MacInv 자신입니다 — 정리 대상에서 뺍니다.\n\n지우려면 터미널에서 brew uninstall --cask macinv") }
          }
        }
        .width(min: 160, ideal: 220)
        TableColumn("버전", value: \.version.orDash) { a in Text(a.version ?? "—").foregroundStyle(.secondary) }
          .width(min: 60, ideal: 90)
        TableColumn("출처", value: \.origin.label) { a in Text(a.origin.label) }
          .width(min: 90, ideal: 130)
        TableColumn("합계", value: \.totalKB.orZero) { a in
          HStack(spacing: 3) {
            Text(sizeText(a.totalKB)).monospacedDigit()
            if a.dataPartial { InfoBadge(text: partialHelp(a)) }
          }
        }
        .width(min: 80, ideal: 95)
        TableColumn("번들", value: \.bundleKB.orZero) { a in Text(sizeText(a.bundleKB)).monospacedDigit().foregroundStyle(.secondary) }
          .width(min: 60, ideal: 75)
        TableColumn("데이터", value: \.dataKB.orZero) { a in
          Text(a.dataPartial ? "\(sizeText(a.dataKB)) 이상" : sizeText(a.dataKB)).monospacedDigit().foregroundStyle(.secondary)
        }
        .width(min: 70, ideal: 95)
        TableColumn("위치", value: \.path) { a in Text((a.path as NSString).deletingLastPathComponent).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
          .width(min: 120, ideal: 200)
        TableColumn("번들 ID", value: \.bundleID.orDash) { a in Text(a.bundleID ?? "—").foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
      }
    }
  }
}

// MARK: Homebrew 포뮬러

/// 표 한 줄: 포뮬러 + 이걸 쓰는 것들(역의존).
struct FormulaRow: Identifiable, Hashable {
  let f: BrewFormula
  let users: [String]
  var id: String { f.name }
  var name: String { f.name }
  var version: String { f.version ?? "—" }
  var install: String { f.onRequest ? "직접 설치" : (users.isEmpty ? "의존성 · 아무도 안 씀" : "의존성으로 설치") }
  var userCount: Int { users.count }
  var depCount: Int { f.dependencies.count }
}

struct FormulaeTable: View {
  /// 검색으로 걸러진 것
  let items: [BrewFormula]
  /// 역의존 계산용 전체(걸러진 목록으로 세면 틀린다)
  let all: [BrewFormula]
  enum Filter: Hashable { case all, onRequest, dependency, unused }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\FormulaRow.name)]

  var body: some View {
    let users = BrewGraph.dependents(all)
    let rows = items.map { FormulaRow(f: $0, users: users[$0.name] ?? []) }
    let unused = rows.filter { !$0.f.onRequest && $0.users.isEmpty }
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", rows.count), (.onRequest, "직접 설치", rows.filter(\.f.onRequest).count),
                          (.dependency, "의존성", rows.filter { !$0.f.onRequest }.count), (.unused, "아무도 안 씀", unused.count)],
                selection: $filter)
      if filter == .unused {
        Label("의존성으로 깔렸는데 이제 이걸 쓰는 포뮬러가 없습니다 — brew autoremove 대상입니다.", systemImage: "info.circle")
          .font(.callout).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 12).padding(.bottom, 6)
      }
      Table(rows.filter(match).sorted(using: order), sortOrder: $order) {
        TableColumn("이름", value: \.name) { r in
          HStack(spacing: 4) {
            Text(r.name)
            if !r.users.isEmpty { LockBadge(text: "다른 포뮬러가 이걸 씁니다(\(r.users.count)개): \(r.users.joined(separator: ", "))\n\n지우면 이것들이 깨집니다. 정리할 때는 MacInv가 경고합니다.") }
          }
        }
        .width(min: 140, ideal: 190)
        TableColumn("버전", value: \.version) { r in Text(r.version).foregroundStyle(.secondary) }
          .width(min: 70, ideal: 100)
        TableColumn("설치", value: \.install) { r in
          Text(r.install).foregroundStyle(r.f.onRequest ? Color.primary : (r.users.isEmpty ? Color.orange : Color.secondary))
        }
        .width(min: 110, ideal: 150)
        TableColumn("이걸 쓰는 것", value: \.userCount) { r in
          Text(r.users.isEmpty ? "—" : r.users.joined(separator: ", ")).foregroundStyle(.secondary).lineLimit(1)
        }
        TableColumn("필요한 것", value: \.depCount) { r in
          Text(r.f.dependencies.isEmpty ? "—" : r.f.dependencies.joined(separator: ", ")).foregroundStyle(.secondary).lineLimit(1)
        }
      }
    }
  }

  private func match(_ r: FormulaRow) -> Bool {
    switch filter {
    case .all: return true
    case .onRequest: return r.f.onRequest
    case .dependency: return !r.f.onRequest
    case .unused: return !r.f.onRequest && r.users.isEmpty
    }
  }
}

// MARK: Homebrew 캐스크

extension BrewCask { var versionText: String { version ?? "—" }; var appsText: String { appNames.joined(separator: ", ") } }

struct CasksTable: View {
  let items: [BrewCask]
  @State private var order = [KeyPathComparator(\BrewCask.token)]
  var body: some View {
    Table(items.sorted(using: order), sortOrder: $order) {
      TableColumn("이름", value: \.token)
      TableColumn("버전", value: \.versionText) { c in Text(c.versionText).foregroundStyle(.secondary) }
      TableColumn("설치한 앱", value: \.appsText) { c in Text(c.appNames.isEmpty ? "—" : c.appsText) }
    }
  }
}

// MARK: 패키지

extension PackageItem { var versionText: String { version ?? "—" }; var managerText: String { manager.label }; var pathText: String { path ?? "—" } }

struct PackagesTable: View {
  let items: [PackageItem]
  @State private var filter: PackageManager? = nil
  @State private var order = [KeyPathComparator(\PackageItem.name)]

  var body: some View {
    if items.isEmpty {
      ContentUnavailableView("패키지 없음", systemImage: "cube", description: Text("npm 전역·pipx·cargo·Go·App Store(mas)로 설치한 것이 없거나, 그 도구가 없습니다."))
    } else {
      let present = PackageManager.allCases.filter { m in items.contains { $0.manager == m } }
      VStack(spacing: 0) {
        FilterBar(options: [(nil, "전체", items.count)] + present.map { m in (Optional(m), m.label, items.filter { $0.manager == m }.count) },
                  selection: $filter)
        Table(items.filter { filter == nil || $0.manager == filter }.sorted(using: order), sortOrder: $order) {
          TableColumn("이름", value: \.name)
          TableColumn("버전", value: \.versionText) { p in Text(p.versionText).foregroundStyle(.secondary) }
          TableColumn("관리자", value: \.managerText) { p in Text(p.managerText) }
          TableColumn("경로", value: \.pathText) { p in Text(p.pathText).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
        }
      }
    }
  }
}

// MARK: 백그라운드 항목

extension BackgroundItem {
  var nameText: String { label ?? ((file as NSString).lastPathComponent as NSString).deletingPathExtension }
  var ownerText: String { owner.label }
  var kindText: String { kind.label }
  var programText: String { program ?? "—" }
  var needsAttention: Bool { owner == .unknown || programMissing || isEmpty }
  var stateText: String {
    if isEmpty { return "내용 없음" }
    if programMissing { return "실행 파일 없음" }
    return runAtLoad ? "로그인 시 실행" : "필요할 때 실행"
  }
}

struct BackgroundTable: View {
  let items: [BackgroundItem]
  enum Filter: Hashable { case all, attention, apple }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\BackgroundItem.nameText)]

  var body: some View {
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", items.count), (.attention, "주의 필요", items.filter(\.needsAttention).count),
                          (.apple, "Apple", items.filter { $0.owner == .apple }.count)], selection: $filter)
      Table(items.filter(match).sorted(using: order), sortOrder: $order) {
        TableColumn("이름", value: \.nameText) { b in
          HStack(spacing: 4) {
            if b.needsAttention { InfoBadge(text: help(b)) }
            if b.owner == .apple { LockBadge(text: "macOS 구성 요소입니다 — MacInv는 건드리지 않습니다.") }
            Text(b.nameText)
          }
        }
        .width(min: 200, ideal: 260)
        TableColumn("주인", value: \.ownerText) { b in Text(b.ownerText).foregroundStyle(b.owner == .unknown ? .orange : .primary) }
          .width(min: 120, ideal: 170)
        TableColumn("상태", value: \.stateText) { b in Text(b.stateText).foregroundStyle(b.programMissing || b.isEmpty ? .orange : .secondary) }
          .width(min: 90, ideal: 120)
        TableColumn("종류", value: \.kindText) { b in Text(b.kindText).foregroundStyle(.secondary) }
          .width(min: 90, ideal: 110)
        TableColumn("실행 파일", value: \.programText) { b in Text(b.programText).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
      }
    }
  }

  private func match(_ b: BackgroundItem) -> Bool {
    switch filter { case .all: return true; case .attention: return b.needsAttention; case .apple: return b.owner == .apple }
  }

  private func help(_ b: BackgroundItem) -> String {
    var lines: [String] = []
    if b.isEmpty { lines.append("내용이 빈 plist({})입니다 — 아무것도 실행하지 않는 껍데기입니다. 앱이 옛 구성요소를 끄면서 남긴 파일로 보입니다(예: 구글의 옛 Keystone).") }
    if b.programMissing { lines.append("실행 파일이 디스크에 없습니다 — 주인 앱을 지운 뒤 남은 항목일 가능성이 큽니다.") }
    if b.owner == .unknown { lines.append("설치된 어느 앱·brew 포뮬러와도 연결되지 않았습니다(실행 파일 위치, 이름·번들 ID, 서명 개발사로 찾음).") }
    lines.append("파일: " + b.file.replacingOccurrences(of: NSHomeDirectory(), with: "~"))
    return lines.joined(separator: "\n\n")
  }
}

// MARK: 업데이트

extension OutdatedItem {
  var sourceText: String { source.label }
  /// 구분 정렬용: 런타임 메이저 → 일반 → 고정
  var kindRank: Int { pinned ? 2 : (runtimeMajor ? 0 : 1) }
}

struct UpdatesView: View {
  let items: [OutdatedItem]?
  let checking: Bool
  let warnings: [String]
  enum Filter: Hashable { case all, normal, major, pinned }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\OutdatedItem.kindRank), KeyPathComparator(\OutdatedItem.name)]

  var body: some View {
    if let items {
      if items.isEmpty {
        ContentUnavailableView("모두 최신입니다", systemImage: "checkmark.circle", description: Text("brew · npm 전역 · App Store(mas)에 업데이트할 것이 없습니다."))
      } else {
        VStack(alignment: .leading, spacing: 0) {
          FilterBar(options: [(.all, "전체", items.count), (.normal, "일반", items.filter { $0.kindRank == 1 }.count),
                              (.major, "런타임 메이저", items.filter { $0.kindRank == 0 }.count),
                              (.pinned, "고정", items.filter(\.pinned).count)], selection: $filter)
          if items.contains(where: \.runtimeMajor) && (filter == .all || filter == .major) {
            Label("런타임 메이저 업그레이드는 프로젝트를 깨뜨릴 수 있어 \"전부 업데이트\"에서 뺍니다. 필요하면 하나씩 고르세요.", systemImage: "exclamationmark.triangle.fill")
              .foregroundStyle(.orange).font(.callout).padding(.horizontal, 12).padding(.bottom, 6)
          }
          Table(items.filter(match).sorted(using: order), sortOrder: $order) {
            TableColumn("이름", value: \.name) { o in Text(o.name) }.width(min: 140, ideal: 200)
            TableColumn("출처", value: \.sourceText) { o in Text(o.sourceText).foregroundStyle(.secondary) }.width(min: 70, ideal: 90)
            TableColumn("현재", value: \.current) { o in Text(o.current).foregroundStyle(.secondary).monospacedDigit() }.width(min: 70, ideal: 100)
            TableColumn("최신", value: \.latest) { o in Text(o.latest).monospacedDigit() }.width(min: 70, ideal: 100)
            TableColumn("구분", value: \.kindRank) { o in kind(o) }
          }
        }
      }
    } else if checking {
      ProgressView("업데이트 확인 중…").frame(maxWidth: .infinity, maxHeight: .infinity)
    } else {
      ContentUnavailableView("업데이트를 확인하지 못했습니다", systemImage: "exclamationmark.triangle", description: Text(warnings.joined(separator: "\n")))
    }
  }

  private func match(_ o: OutdatedItem) -> Bool {
    switch filter { case .all: return true; case .normal: return o.kindRank == 1; case .major: return o.kindRank == 0; case .pinned: return o.pinned }
  }

  @ViewBuilder private func kind(_ o: OutdatedItem) -> some View {
    if o.pinned {
      Text("고정됨(brew pin)").foregroundStyle(.secondary)
    } else if o.runtimeMajor {
      HStack(spacing: 4) {
        InfoBadge(text: "\(o.name) \(o.current) → \(o.latest)는 언어 런타임의 메이저 업그레이드입니다.\n\n이 버전에 맞춰 둔 프로젝트가 깨질 수 있어 \"전부 업데이트\"에서는 빼고, 직접 고를 때만 실행합니다.")
        Text("런타임 메이저")
      }
    } else {
      Text("일반").foregroundStyle(.secondary)
    }
  }
}

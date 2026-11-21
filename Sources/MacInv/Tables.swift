import SwiftUI
import MacInvCore

// 목록 표들 — 모든 열 제목을 눌러 정렬, 표 위 분류 버튼으로 나눠 보기,
// 건드리면 안 되는 것(애플 기본 앱·시스템 런타임·MacInv 자신·다른 것이 쓰는 포뮬러)은 🔒와 이유.

/// 표 위 분류 버튼(개수 포함).
struct FilterBar<F: Hashable>: View {
  let options: [(value: F, title: String, count: Int)]
  @Binding var selection: F
  /// 오른쪽 계획 버튼들
  var plans: [PlanButton] = []
  var body: some View {
    HStack {
      if !options.isEmpty {
        Picker("보기", selection: $selection) {
          ForEach(options, id: \.value) { o in Text("\(o.title) \(o.count)").tag(o.value) }
        }
        .pickerStyle(.segmented)
        .labelsHidden()
        .fixedSize()
      }
      Spacer()
      ForEach(plans.indices, id: \.self) { plans[$0] }
    }
    .padding(.horizontal, 12).padding(.vertical, 8)
  }
}

/// 고른 줄의 삭제 계획 보기(읽기 전용). 더블클릭·엔터에는 묶지 않는다 — 기본 동작이 삭제로 이어지면 안 된다.
struct PlanButton: View {
  var title = "삭제…"
  var symbol = "trash"
  var hint = "실행 전에 무엇을 지우고 무엇을 남기는지 확인 화면을 띄웁니다"
  let action: (() -> Void)?
  var body: some View {
    Button { action?() } label: { Label(title, systemImage: symbol) }
      .disabled(action == nil)
      .help(action == nil ? "표에서 고르세요" : hint)
  }
}

/// 표 줄 오른쪽 클릭 메뉴
func planMenu(_ run: (() -> Void)?, title: String = "삭제…") -> some View {
  Group { if let run { Button(title, action: run) } }
}

/// 표 줄 오른쪽 클릭 메뉴: 업데이트(있을 때) + 삭제
func rowMenu(update: (() -> Void)?, delete: (() -> Void)?) -> some View {
  Group {
    if let update { Button("업데이트…", action: update) }
    if let delete { Button("삭제…", action: delete) }
  }
}

/// 「최신」 칸 — 업데이트가 있으면 새 버전(파랑), 런타임 메이저는 주황 + 설명, 고정은 회색
struct LatestCell: View {
  let item: OutdatedItem?
  var body: some View {
    if let u = item {
      HStack(spacing: 3) {
        if u.pinned {
          Text("\(u.latest) · 고정됨").foregroundStyle(.secondary)
        } else if u.runtimeMajor {
          Text("→ \(u.latest)").foregroundStyle(.orange).monospacedDigit()
          InfoBadge(text: "\(u.name) \(u.current) → \(u.latest)는 언어 런타임의 메이저 업그레이드입니다.\n\n이 버전에 맞춰 둔 프로젝트가 깨질 수 있어 「모두 업데이트」에서는 빼고, 직접 고를 때만 실행합니다.")
        } else {
          Text("→ \(u.latest)").foregroundStyle(.blue).monospacedDigit()
        }
      }
    } else {
      Text("—").foregroundStyle(.tertiary)
    }
  }
}

extension Optional where Wrapped == OutdatedItem {
  /// 정렬용: 업데이트 있는 것이 먼저 오게(빈 값은 맨 뒤)
  var latestKey: String { map { $0.pinned ? "1" + $0.latest : "0" + $0.latest } ?? "2" }
}

extension AppModel {
  /// 이 출처·이름의 업데이트 대기(없으면 nil)
  func update(_ source: OutdatedItem.Source, _ name: String) -> OutdatedItem? {
    outdated?.first { $0.source == source && $0.name == name }
  }

  /// 한 항목 업데이트 확인 화면을 여는 동작(고정된 것은 nil — 버튼이 꺼진다)
  func updateAction(_ u: OutdatedItem?) -> (() -> Void)? {
    guard let u, !u.pinned else { return nil }
    return { self.showPlan { Planner.upgrade([u], everything: false, inventory: $0, env: $1) } }
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
  @State private var selection: AppItem.ID?
  @EnvironmentObject private var model: AppModel
  private let me = SelfIdentity.current()

  private func planAction(_ id: AppItem.ID?) -> (() -> Void)? {
    guard let id, let a = apps.first(where: { $0.id == id }) else { return nil }
    return { model.showPlan { Planner.removal(app: a, inventory: $0, env: $1) } }
  }

  private func kind(_ a: AppItem) -> Filter {
    switch a.origin { case .apple: return .apple; case .appStore: return .appStore; case .brewCask: return .brew; case .other: return .other }
  }

  var body: some View {
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", apps.count), (.other, "직접 설치", apps.filter { kind($0) == .other }.count),
                          (.appStore, "App Store", apps.filter { kind($0) == .appStore }.count),
                          (.brew, "Homebrew", apps.filter { kind($0) == .brew }.count),
                          (.apple, "Apple(보호)", apps.filter { kind($0) == .apple }.count)], selection: $filter,
                plans: [PlanButton(action: planAction(selection))])
      Table(apps.filter { filter == .all || kind($0) == filter }.sorted(using: order), selection: $selection, sortOrder: $order) {
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
      .contextMenu(forSelectionType: AppItem.ID.self) { ids in planMenu(planAction(ids.first)) }
    }
  }
}

// MARK: Homebrew 포뮬러

/// 표 한 줄: 포뮬러 + 이걸 쓰는 것들(역의존).
struct FormulaRow: Identifiable, Hashable {
  let f: BrewFormula
  let users: [String]
  /// 업데이트 대기(없으면 nil)
  let upd: OutdatedItem?
  var latestKey: String { upd.latestKey }
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
  enum Filter: Hashable { case all, outdated, onRequest, dependency, unused }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\FormulaRow.name)]
  @State private var selection: FormulaRow.ID?
  @EnvironmentObject private var model: AppModel

  private func planAction(_ id: FormulaRow.ID?) -> (() -> Void)? {
    guard let id, let f = all.first(where: { $0.name == id }) else { return nil }
    return { model.showPlan { Planner.removal(formula: f, inventory: $0, env: $1) } }
  }

  var body: some View {
    let users = BrewGraph.dependents(all)
    let rows = items.map { FormulaRow(f: $0, users: users[$0.name] ?? [], upd: model.update(.brewFormula, $0.name)) }
    let unused = rows.filter { !$0.f.onRequest && $0.users.isEmpty }
    let chosen = rows.first { $0.id == selection }
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", rows.count), (.outdated, "업데이트 있음", rows.filter { $0.upd != nil }.count),
                          (.onRequest, "직접 설치", rows.filter(\.f.onRequest).count),
                          (.dependency, "의존성", rows.filter { !$0.f.onRequest }.count), (.unused, "아무도 안 씀", unused.count)],
                selection: $filter, plans: [PlanButton(title: "업데이트…", symbol: "arrow.up.circle", hint: "고른 포뮬러를 업데이트합니다",
                                                       action: model.updateAction(chosen?.upd)),
                                            PlanButton(action: planAction(selection))])
      if filter == .unused {
        Label("의존성으로 깔렸는데 이제 이걸 쓰는 포뮬러가 없습니다 — brew autoremove 대상입니다.", systemImage: "info.circle")
          .font(.callout).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 12).padding(.bottom, 6)
      }
      Table(rows.filter(match).sorted(using: order), selection: $selection, sortOrder: $order) {
        TableColumn("이름", value: \.name) { r in
          HStack(spacing: 4) {
            Text(r.name)
            if !r.users.isEmpty { LockBadge(text: "다른 포뮬러가 이걸 씁니다(\(r.users.count)개): \(r.users.joined(separator: ", "))\n\n지우면 이것들이 깨집니다. 정리할 때는 MacInv가 경고합니다.") }
          }
        }
        .width(min: 140, ideal: 190)
        TableColumn("버전", value: \.version) { r in Text(r.version).foregroundStyle(.secondary) }
          .width(min: 70, ideal: 100)
        TableColumn("최신", value: \.latestKey) { r in LatestCell(item: r.upd) }
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
      .contextMenu(forSelectionType: FormulaRow.ID.self) { ids in
        rowMenu(update: model.updateAction(rows.first { $0.id == ids.first }?.upd), delete: planAction(ids.first))
      }
    }
  }

  private func match(_ r: FormulaRow) -> Bool {
    switch filter {
    case .all: return true
    case .outdated: return r.upd != nil
    case .onRequest: return r.f.onRequest
    case .dependency: return !r.f.onRequest
    case .unused: return !r.f.onRequest && r.users.isEmpty
    }
  }
}

// MARK: Homebrew 캐스크

extension BrewCask { var versionText: String { version ?? "—" }; var appsText: String { appNames.joined(separator: ", ") } }

struct CaskRow: Identifiable, Hashable {
  let c: BrewCask
  let upd: OutdatedItem?
  var id: String { c.token }
  var token: String { c.token }
  var versionText: String { c.versionText }
  var appsText: String { c.appsText }
  var latestKey: String { upd.latestKey }
}

struct CasksTable: View {
  let items: [BrewCask]
  enum Filter: Hashable { case all, outdated }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\CaskRow.token)]
  @State private var selection: CaskRow.ID?
  @EnvironmentObject private var model: AppModel
  private let me = SelfIdentity.current()

  private func planAction(_ id: CaskRow.ID?) -> (() -> Void)? {
    guard let id, let c = items.first(where: { $0.id == id }) else { return nil }
    return { model.showPlan { Planner.removal(cask: c, inventory: $0, env: $1) } }
  }

  var body: some View {
    let rows = items.map { CaskRow(c: $0, upd: model.update(.brewCask, $0.token)) }
    let chosen = rows.first { $0.id == selection }
    VStack(spacing: 0) {
      FilterBar(options: [(.all, "전체", rows.count), (.outdated, "업데이트 있음", rows.filter { $0.upd != nil }.count)], selection: $filter,
                plans: [PlanButton(title: "업데이트…", symbol: "arrow.up.circle", hint: "고른 앱을 업데이트합니다(brew upgrade --cask)",
                                   action: model.updateAction(chosen?.upd)),
                        PlanButton(action: planAction(selection))])
      Table(rows.filter { filter == .all || $0.upd != nil }.sorted(using: order), selection: $selection, sortOrder: $order) {
        TableColumn("이름", value: \.token) { r in
          HStack(spacing: 4) {
            Text(r.token)
            if me.isSelf(caskToken: r.token) { LockBadge(text: "MacInv 자신입니다 — 정리 대상에서 뺍니다.\n\n지우려면 터미널에서 brew uninstall --cask macinv") }
          }
        }
        TableColumn("버전", value: \.versionText) { r in Text(r.versionText).foregroundStyle(.secondary) }
        TableColumn("최신", value: \.latestKey) { r in LatestCell(item: r.upd) }
        TableColumn("설치한 앱", value: \.appsText) { r in Text(r.c.appNames.isEmpty ? "—" : r.appsText) }
      }
      .contextMenu(forSelectionType: CaskRow.ID.self) { ids in
        rowMenu(update: model.updateAction(rows.first { $0.id == ids.first }?.upd), delete: planAction(ids.first))
      }
    }
  }
}

// MARK: 패키지

extension PackageItem { var versionText: String { version ?? "—" }; var managerText: String { manager.label }; var pathText: String { path ?? "—" } }

struct PackageRow: Identifiable, Hashable {
  let p: PackageItem
  let upd: OutdatedItem?
  var id: String { p.id }
  var name: String { p.name }
  var versionText: String { p.versionText }
  var managerText: String { p.managerText }
  var pathText: String { p.pathText }
  var latestKey: String { upd.latestKey }
}

struct PackagesTable: View {
  let items: [PackageItem]
  enum Filter: Hashable { case all, outdated, manager(PackageManager) }
  @State private var filter = Filter.all
  @State private var order = [KeyPathComparator(\PackageRow.name)]
  @State private var selection: PackageRow.ID?
  @EnvironmentObject private var model: AppModel

  private func planAction(_ id: PackageRow.ID?) -> (() -> Void)? {
    guard let id, let p = items.first(where: { $0.id == id }) else { return nil }
    return { model.showPlan { Planner.removal(package: p, inventory: $0, env: $1) } }
  }

  /// 업데이트 대기는 npm 전역과 App Store만 안다(pipx·cargo·Go는 확인하지 않음)
  private func upd(_ p: PackageItem) -> OutdatedItem? {
    switch p.manager {
    case .npm: return model.update(.npm, p.name)
    case .mas: return model.update(.mas, p.name)
    default: return nil
    }
  }

  var body: some View {
    if items.isEmpty {
      ContentUnavailableView("패키지 없음", systemImage: "cube", description: Text("npm 전역·pipx·cargo·Go·App Store(mas)로 설치한 것이 없거나, 그 도구가 없습니다."))
    } else {
      let rows = items.map { PackageRow(p: $0, upd: upd($0)) }
      let chosen = rows.first { $0.id == selection }
      let present = PackageManager.allCases.filter { m in items.contains { $0.manager == m } }
      VStack(spacing: 0) {
        FilterBar(options: [(Filter.all, "전체", rows.count), (.outdated, "업데이트 있음", rows.filter { $0.upd != nil }.count)]
                    + present.map { m in (Filter.manager(m), m.label, items.filter { $0.manager == m }.count) },
                  selection: $filter,
                  plans: [PlanButton(title: "업데이트…", symbol: "arrow.up.circle", hint: "고른 패키지를 업데이트합니다(npm 전역·App Store)",
                                     action: model.updateAction(chosen?.upd)),
                          PlanButton(action: planAction(selection))])
        Table(rows.filter(match).sorted(using: order), selection: $selection, sortOrder: $order) {
          TableColumn("이름", value: \.name)
          TableColumn("버전", value: \.versionText) { r in Text(r.versionText).foregroundStyle(.secondary) }
          TableColumn("최신", value: \.latestKey) { r in LatestCell(item: r.upd) }
          TableColumn("관리자", value: \.managerText) { r in Text(r.managerText) }
          TableColumn("경로", value: \.pathText) { r in Text(r.pathText).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle) }
        }
        .contextMenu(forSelectionType: PackageRow.ID.self) { ids in
          rowMenu(update: model.updateAction(rows.first { $0.id == ids.first }?.upd), delete: planAction(ids.first))
        }
      }
    }
  }

  private func match(_ r: PackageRow) -> Bool {
    switch filter {
    case .all: return true
    case .outdated: return r.upd != nil
    case .manager(let m): return r.p.manager == m
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
  @State private var selection = Set<OutdatedItem.ID>()
  @EnvironmentObject private var model: AppModel

  private func planAction(_ ids: Set<OutdatedItem.ID>, everything: Bool = false) -> (() -> Void)? {
    let chosen = everything ? (items ?? []) : (items ?? []).filter { ids.contains($0.id) }
    guard !chosen.isEmpty else { return nil }
    return { model.showPlan { Planner.upgrade(chosen, everything: everything, inventory: $0, env: $1) } }
  }

  var body: some View {
    if let items {
      if items.isEmpty {
        ContentUnavailableView("모두 최신입니다", systemImage: "checkmark.circle",
                               description: Text((["brew · npm 전역 · App Store(mas)에 업데이트할 것이 없습니다."] + warnings).joined(separator: "\n")))
      } else {
        VStack(alignment: .leading, spacing: 0) {
          FilterBar(options: [(.all, "전체", items.count), (.normal, "일반", items.filter { $0.kindRank == 1 }.count),
                              (.major, "런타임 메이저", items.filter { $0.kindRank == 0 }.count),
                              (.pinned, "고정", items.filter(\.pinned).count)], selection: $filter,
                    plans: [PlanButton(title: selection.isEmpty ? "선택한 것 업데이트…" : "선택한 \(selection.count)개 업데이트…", symbol: "checklist",
                                       hint: "⌘-클릭으로 여러 개, ⇧-클릭으로 범위, ⌘A로 전부 고를 수 있습니다", action: planAction(selection)),
                            PlanButton(title: "모두 업데이트…", symbol: "arrow.up.circle",
                                       hint: "런타임 메이저·고정된 것·암호가 필요한 것은 빼고 업데이트합니다", action: planAction([], everything: true))])
          ForEach(warnings, id: \.self) { w in
            Label(w, systemImage: "exclamationmark.triangle").foregroundStyle(.secondary).font(.callout).padding(.horizontal, 12).padding(.bottom, 6)
          }
          if items.contains(where: \.runtimeMajor) && (filter == .all || filter == .major) {
            Label("런타임 메이저 업그레이드는 프로젝트를 깨뜨릴 수 있어 \"전부 업데이트\"에서 뺍니다. 필요하면 하나씩 고르세요.", systemImage: "exclamationmark.triangle.fill")
              .foregroundStyle(.orange).font(.callout).padding(.horizontal, 12).padding(.bottom, 6)
          }
          Table(items.filter(match).sorted(using: order), selection: $selection, sortOrder: $order) {
            TableColumn("이름", value: \.name) { o in Text(o.name) }.width(min: 140, ideal: 200)
            TableColumn("출처", value: \.sourceText) { o in Text(o.sourceText).foregroundStyle(.secondary) }.width(min: 70, ideal: 90)
            TableColumn("현재", value: \.current) { o in Text(o.current).foregroundStyle(.secondary).monospacedDigit() }.width(min: 70, ideal: 100)
            TableColumn("최신", value: \.latest) { o in Text(o.latest).monospacedDigit() }.width(min: 70, ideal: 100)
            TableColumn("구분", value: \.kindRank) { o in kind(o) }
          }
          .contextMenu(forSelectionType: OutdatedItem.ID.self) { ids in planMenu(planAction(ids), title: "업데이트…") }
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

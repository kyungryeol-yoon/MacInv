import SwiftUI
import MacInvCore

/// 목록 창: 왼쪽 분류, 오른쪽 표.
struct MainWindow: View {
  @EnvironmentObject private var model: AppModel
  @State private var section: Section? = .apps
  @State private var query = ""

  enum Section: String, Hashable, CaseIterable {
    case apps, updates, disk, formulae, casks, packages, runtimes, background, environment
    var title: String {
      switch self {
      case .apps: return "앱"
      case .updates: return "업데이트"
      case .disk: return "디스크"
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
      case .updates: return "arrow.up.circle"
      case .disk: return "internaldrive"
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
      .onChange(of: model.requestedSection, initial: true) { _, s in
        if let s { section = s; model.requestedSection = nil }
      }
    } detail: {
      Group {
        if model.inventory == nil {
          ProgressView("스캔 중…").frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
          switch section ?? .apps {
          case .apps: AppsTable(apps: filtered(model.inventory!.apps) { $0.name + " " + ($0.bundleID ?? "") })
          case .disk: DiskView(report: model.disk, scanning: model.scanningDisk, query: query)
          case .updates: UpdatesView(items: model.outdated.map { filtered($0) { $0.name } }, checking: model.checkingUpdates, warnings: model.outdatedWarnings)
          case .formulae: FormulaeTable(items: filtered(model.inventory!.formulae) { $0.name }, all: model.inventory!.formulae)
          case .casks: CasksTable(items: filtered(model.inventory!.casks) { $0.token + " " + $0.appNames.joined(separator: " ") })
          case .packages: PackagesTable(items: filtered(model.inventory!.packages) { $0.name })
          case .runtimes: RuntimesView(runtimes: filtered(model.inventory!.runtimes) { $0.name }, managers: model.inventory!.versionManagers,
                                       eol: model.runtimeEOL, hasMise: model.hasMise)
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
    case .updates: return model.outdated == nil ? nil : model.pendingUpdates
    case .disk: return model.disk?.entries.count
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
  f.allowedUnits = [.useKB, .useMB, .useGB, .useTB]   // 0은 "0 bytes"가 아니라 "0 KB"
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
  let eol: [String: RuntimeEOL]
  let hasMise: Bool
  var body: some View {
    VSplitView {
      Table(runtimes) {
        TableColumn("언어") { r in Text(r.name).foregroundStyle(r.installed ? .primary : .secondary) }
          .width(min: 70, ideal: 80)
        TableColumn("버전") { r in Text(r.installed ? (r.version ?? "?") : "설치 안 됨").foregroundStyle(r.installed ? .primary : .secondary) }
          .width(min: 70, ideal: 90)
        TableColumn("관리") { r in
          if let a = r.active {
            HStack(spacing: 4) {
              Text(a.manager.label + (a.package.map { " \($0)" } ?? ""))
              if a.manager == .system { LockBadge(text: "macOS에 딸린 \(r.name)입니다 — 건드리지 않습니다.\n\n시스템 도구가 이 버전에 기대고 있을 수 있어 제자리 업그레이드·삭제를 하지 않습니다. 새 버전이 필요하면 따로 설치해 PATH 앞에 둡니다.") }
            }
          } else { Text("—").foregroundStyle(.secondary) }
        }
        .width(min: 80, ideal: 110)
        TableColumn("지원") { r in support(r) }
          .width(min: 110, ideal: 150)
        TableColumn("업데이트") { r in update(r) }
          .width(min: 120, ideal: 170)
        TableColumn("쓰이는 경로") { r in
          HStack(spacing: 4) {
            if r.conflict { InfoBadge(text: conflictHelp(r)) }
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

  @ViewBuilder private func support(_ r: Runtime) -> some View {
    if let e = eol[r.name] {
      switch e.status {
      case .passed: Text("EOL 지남" + (e.eolDate.map { " \($0)" } ?? "")).foregroundStyle(.red)
      case .soon:
        Text(e.eolDate.flatMap { EOL.monthsUntil($0) }.map { "\($0)개월 뒤 종료" } ?? "곧 종료").foregroundStyle(.orange)
      case .supported, .rolling: Text("지원 중").foregroundStyle(.secondary)
      case .unknown: Text("—").foregroundStyle(.secondary)
      }
    } else {
      Text(r.installed && EOL.products[r.name] != nil ? "—" : "").foregroundStyle(.secondary)
    }
  }

  @ViewBuilder private func update(_ r: Runtime) -> some View {
    if let e = eol[r.name] {
      HStack(spacing: 4) {
        switch e.update {
        case .current: Text("현재").foregroundStyle(.secondary)
        case .patch: Text("안전한 패치 \(e.latestVersion)")
        case .major:
          // go/rust는 1.x 안에서 하위 호환을 정책으로 보장 — 업데이트 목록과 같은 판정으로 경고색을 뺀다
          if Updates.runtimeMajorJump(name: r.name, current: r.version ?? "", latest: e.latestCycle) {
            Text("메이저 최신은 \(e.latestCycle)").foregroundStyle(.orange)
          } else {
            Text("최신 사이클 \(e.latestCycle) (하위 호환)").foregroundStyle(.secondary)
          }
        case .unknown: Text("—").foregroundStyle(.secondary)
        }
        if e.update != .current, let a = r.active,
           let cmd = EOL.suggestCommand(runtime: r.name, manager: a.manager, latestCycle: e.latestCycle, hasMise: hasMise) {
          InfoBadge(systemImage: "terminal", color: .secondary, text: commandHelp(r, e, cmd))
        }
      }
    }
  }

  private func commandHelp(_ r: Runtime, _ e: RuntimeEOL, _ cmd: String) -> String {
    var s = e.update == .major
      ? "\(r.name) \(r.version ?? "")은 \(e.cycle) 사이클입니다. 최신 사이클은 \(e.latestCycle)입니다.\n메이저를 올리면 이 버전에 맞춰 둔 프로젝트가 깨질 수 있어 MacInv가 대신 실행하지 않습니다."
      : "\(r.name) \(r.version ?? "") → \(e.latestVersion)(같은 \(e.cycle) 사이클 안의 패치)."
    if e.update == .major, e.latestVersion != (r.version ?? "") { s += "\n같은 사이클의 최신 패치는 \(e.latestVersion)입니다." }
    s += "\n\n직접 실행할 명령:\n\(cmd)"
    return s
  }

  private func conflictHelp(_ r: Runtime) -> String {
    "PATH에 서로 다른 \(r.name)가 \(Set(r.origins.map(\.resolved)).count)개 있습니다 — 맨 위 것이 쓰입니다:\n" +
      r.origins.map { "\($0.path) (\($0.manager.label))" }.joined(separator: "\n")
  }
}

/// ⚠️ 아이콘 — 마우스를 올리거나 누르면 설명 창.
/// 표(Table) 칸 안에서는 `.help` 툴팁이 뜨지 않아서(2026-11-21 실측) 팝오버로 보여 준다.
struct InfoBadge: View {
  var systemImage = "exclamationmark.triangle.fill"
  var color: Color = .orange
  let text: String
  @State private var shown = false

  var body: some View {
    Image(systemName: systemImage)
      .foregroundStyle(color)
      .imageScale(.small)
      .padding(3)
      .contentShape(Rectangle())
      .onHover { shown = $0 }
      .onTapGesture { shown.toggle() }
      .popover(isPresented: $shown, arrowEdge: .bottom) {
        // 표 칸의 한 줄 제한(lineLimit)이 팝오버 안까지 따라 들어와 잘렸다 — 여기서 풀어 준다
        Text(text)
          .font(.callout)
          .lineLimit(nil)
          .multilineTextAlignment(.leading)
          .textSelection(.enabled)
          .frame(width: 380, alignment: .leading)
          .fixedSize(horizontal: false, vertical: true)
          .padding(14)
      }
      .accessibilityLabel(Text(text))
  }
}

struct DiskView: View {
  let report: DiskReport?
  let scanning: Bool
  let query: String
  @State private var order = [KeyPathComparator(\DiskEntry.sizeKB, order: .reverse)]
  @State private var tier: Tier? = nil

  var body: some View {
    if let r = report {
      VStack(alignment: .leading, spacing: 0) {
        summary(r).padding(12)
        notices(r)
        Divider()
        FilterBar(options: [(nil, "전체", r.entries.count)] + [Tier.safe, .artifact, .expensive].compactMap { t in
          let n = r.entries.filter { $0.location.tier == t }.count
          return n > 0 ? (Optional(t), t.label, n) : nil
        }, selection: $tier)
        Table(rows(r), sortOrder: $order) {
          TableColumn("위치", value: \.location.name) { e in
            VStack(alignment: .leading, spacing: 2) {
              Text(e.location.name)
              if !e.breakdown.isEmpty {
                Text(breakdownLine(e.breakdown)).font(.caption).foregroundStyle(.secondary).lineLimit(1)
              }
            }
          }
          .width(min: 220, ideal: 320)
          TableColumn("용량", value: \.sizeKB) { e in
            HStack(spacing: 3) {
              Text(sizeText(e.sizeKB)).monospacedDigit()
              if e.partial { InfoBadge(text: partialText(e)) }
            }
          }
          .width(min: 80, ideal: 100)
          TableColumn("등급") { e in Text(e.location.tier.label).foregroundStyle(e.location.tier == .expensive ? .orange : .secondary) }
            .width(min: 90, ideal: 110)
          TableColumn("상태") { e in status(e) }
          .width(min: 110, ideal: 150)
          TableColumn("정리 방법") { e in
            HStack(spacing: 4) {
              Text(method(e)).foregroundStyle(.secondary).lineLimit(1)
              if let note = e.location.note { InfoBadge(systemImage: "info.circle", color: .secondary, text: note) }
            }
          }
        }
        if let models = r.ollamaModels, !models.isEmpty { ollama(models, r).padding(12) }
      }
    } else {
      ProgressView(scanning ? "디스크 재는 중…" : "인벤토리를 먼저 스캔합니다…").frame(maxWidth: .infinity, maxHeight: .infinity)
    }
  }

  @ViewBuilder private func status(_ e: DiskEntry) -> some View {
    HStack(spacing: 4) {
      if e.orphan { InfoBadge(text: orphanText(e)); Text("고아 캐시").foregroundStyle(.orange) }
      else if let (p, t) = e.linkedTo.first {
        InfoBadge(systemImage: "arrow.turn.down.right", color: .secondary, text: linkText(p, t))
        Text(Disk.externalVolume(t).map { "→ \($0)" } ?? "→ 다른 위치").foregroundStyle(.secondary).lineLimit(1)
      } else if !e.blockedByRunning.isEmpty {
        InfoBadge(systemImage: "pause.circle.fill", color: .secondary, text: blockedText(e))
        Text("일부 실행 중").foregroundStyle(.secondary)
      }
    }
  }

  @ViewBuilder private func notices(_ r: DiskReport) -> some View {
    VStack(alignment: .leading, spacing: 6) {
      if r.runningCheckFailed {
        Label("실행 중인 앱을 확인하지 못했습니다 — 실행 중인 앱의 캐시 보호가 꺼진 상태입니다. 다시 스캔해 주세요.", systemImage: "exclamationmark.triangle.fill")
          .foregroundStyle(.orange)
      }
      let blocked = r.blockedByApp
      if !blocked.isEmpty {
        Label("⏸ \(blocked.count)개 앱이 실행 중이라 정리에서 뺐습니다(총 \(sizeText(blocked.reduce(0) { $0 + $1.kb }))): " +
              blocked.prefix(4).map { "\($0.name) \(sizeText($0.kb))" }.joined(separator: " · ") + " — 앱을 끄고 다시 스캔하면 포함됩니다.",
              systemImage: "pause.circle")
          .foregroundStyle(.secondary)
      }
      if let s = r.snapshots, !s.isEmpty {
        HStack(spacing: 4) {
          Label("Time Machine 로컬 스냅샷 \(s.count)개 — 지운 공간이 바로 여유 공간으로 돌아오지 않을 수 있습니다.", systemImage: "clock.arrow.circlepath")
            .foregroundStyle(.secondary)
          InfoBadge(systemImage: "info.circle", color: .secondary, text: "macOS는 지운 파일을 로컬 스냅샷이 계속 붙잡고 있을 수 있습니다. 공간이 필요하면 macOS가 알아서 스냅샷을 줄입니다.\n\n스냅샷을 지우면 그 시점으로 되돌릴 수 없게 되므로 MacInv는 지우지 않고 알려만 드립니다.")
        }
      }
    }
    .font(.callout)
    .padding(.horizontal, 12)
    .padding(.bottom, r.runningCheckFailed || !r.blockedByApp.isEmpty || !(r.snapshots ?? []).isEmpty ? 10 : 0)
  }

  private func ollama(_ models: [OllamaModel], _ r: DiskReport) -> some View {
    let link = r.entries.first { $0.location.id == "ollama-models" }?.linkedTo.values.first
    return VStack(alignment: .leading, spacing: 6) {
      HStack {
        Text("Ollama 모델 \(models.count)개 · \(sizeText(models.reduce(0) { $0 + $1.sizeKB }))").font(.callout.weight(.semibold))
        if let link, let vol = Disk.externalVolume(link) { Text("· 외장 디스크(\(vol))에 있음 — 이 맥 공간을 차지하지 않음").font(.callout).foregroundStyle(.secondary) }
        Spacer()
      }
      ForEach(models) { m in
        HStack { Text(m.name).monospaced(); Spacer(); Text(sizeText(m.sizeKB)).monospacedDigit().foregroundStyle(.secondary) }.font(.callout)
      }
      Text("모델은 하나씩 고르게 됩니다(6단계) — 하나가 수 GB이고 다시 받기 비쌉니다.").font(.caption).foregroundStyle(.tertiary)
    }
    .cardBackground()
  }

  private func linkText(_ p: String, _ t: String) -> String {
    let home = NSHomeDirectory()
    let short = { (x: String) in x.hasPrefix(home) ? "~" + x.dropFirst(home.count) : x }
    var s = "\(short(p))는 다른 곳으로 가는 바로가기(심볼릭 링크)입니다.\n→ \(short(t))"
    if let v = Disk.externalVolume(t) { s += "\n\n실제 데이터는 「\(v)」에 있어 이 맥 내장 디스크 공간을 차지하지 않습니다. 그래서 여기 용량은 0에 가깝게 나옵니다." }
    return s
  }

  private func blockedText(_ e: DiskEntry) -> String {
    let apps = Set(e.blockedByRunning.values).sorted()
    return "실행 중인 앱(\(apps.joined(separator: ", ")))의 캐시 \(sizeText(e.blockedKB))는 정리에서 뺍니다.\n\n켜져 있는 동안 캐시를 지우면 앱이 오작동할 수 있습니다. 앱을 끄고 다시 스캔하면 포함됩니다."
  }

  private func rows(_ r: DiskReport) -> [DiskEntry] {
    let q = query.trimmingCharacters(in: .whitespaces)
    let byTier = tier == nil ? r.entries : r.entries.filter { $0.location.tier == tier }
    let base = q.isEmpty ? byTier : byTier.filter { ($0.location.name + " " + $0.breakdown.map(\.name).joined(separator: " ")).localizedCaseInsensitiveContains(q) }
    return base.sorted(using: order)
  }

  @ViewBuilder private func summary(_ r: DiskReport) -> some View {
    HStack(alignment: .firstTextBaseline, spacing: 28) {
      stat("정리하면 늘어날 공간", sizeText(r.reclaimableKB), "안전(재생성) + 빌드 산출물")
      stat("재다운로드 비싼 것", sizeText(r.expensiveKB), "모델·이미지 — 기본 정리에서 뺌")
      stat("알려진 위치 합계", sizeText(r.totalKB), "\(r.entries.count)곳")
      if let free = r.freeBytes, let total = r.totalBytes {
        stat("디스크 여유", ByteCountFormatter.string(fromByteCount: free, countStyle: .file), "전체 \(ByteCountFormatter.string(fromByteCount: total, countStyle: .file))")
      }
      Spacer()
    }
  }

  private func stat(_ title: String, _ value: String, _ sub: String) -> some View {
    VStack(alignment: .leading, spacing: 2) {
      Text(title).font(.caption).foregroundStyle(.secondary)
      Text(value).font(.title3.weight(.semibold)).monospacedDigit()
      Text(sub).font(.caption2).foregroundStyle(.tertiary)
    }
  }

  /// "Claude 1.7 GB · Code 74 MB · 외 3개"
  private func breakdownLine(_ b: [(name: String, kb: Int)]) -> String {
    let top = b.prefix(5).map { "\($0.name) \(sizeText($0.kb))" }.joined(separator: " · ")
    return b.count > 5 ? top + " · 외 \(b.count - 5)개" : top
  }

  private func method(_ e: DiskEntry) -> String {
    if let c = e.location.cleanup, !c.isEmpty { return c.map { $0.joined(separator: " ") }.joined(separator: " && ") }
    if e.location.trash == true { return "휴지통으로 이동" }
    return "직접 정리"
  }

  private func orphanText(_ e: DiskEntry) -> String {
    var s = "이 캐시를 쓰는 도구(\(e.location.tool))"
    if let apps = e.location.owner_apps, !apps.isEmpty { s += "나 앱(\(apps.joined(separator: ", ")))" }
    s += "이 이 맥에 하나도 없습니다 — 도구를 지운 뒤 남은 캐시로 보입니다.\n\n설치 목록·PATH·이 캐시를 쓰는 앱까지 확인했습니다."
    return s
  }

  private func partialText(_ e: DiskEntry) -> String {
    let home = NSHomeDirectory()
    var s = "⚠️ 일부만 측정 — 전체 디스크 접근 권한이 없어 일부 폴더를 읽지 못했습니다. 실제 용량은 이보다 큽니다."
    if !e.deniedPaths.isEmpty {
      s += "\n\n읽지 못한 곳:"
      for p in e.deniedPaths.prefix(6) { s += "\n• " + (p.hasPrefix(home) ? "~" + p.dropFirst(home.count) : p) }
    }
    return s
  }
}

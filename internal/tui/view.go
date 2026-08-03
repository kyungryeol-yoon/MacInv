package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	"github.com/kyungryeol-yoon/macinv/internal/disk"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// listHeight는 목록에 쓸 수 있는 세로 줄 수다(헤더/푸터/로그패널 제외).
func (m Model) listHeight() int {
	h := m.h - 2 /*헤더*/ - 1 /*푸터*/
	if len(m.log) > 0 {
		h -= logViewLines + 1
	}
	if h < 1 {
		return 1
	}
	return h
}

// View는 모드에 따라 화면을 그린다.
func (m Model) View() string {
	if m.h == 0 {
		return "\n  로딩 중…\n"
	}
	switch m.mode {
	case modePlan:
		return m.planView()
	case modeConfirm:
		return m.confirmView()
	case modeBuilding:
		verb := m.buildingVerb
		if verb == "" {
			verb = "삭제"
		}
		return m.header() + "\n\n  ⏳ " + verb + " 계획 작성 중…\n" + m.logPanel()
	default:
		return m.browseView()
	}
}

func (m Model) header() string {
	var tabsRow strings.Builder
	for i, t := range m.tabs {
		label := fmt.Sprintf("%s (%d)", t.title, t.count)
		if i == m.active {
			tabsRow.WriteString(activeTab.Render(label))
		} else {
			tabsRow.WriteString(inactiveTab.Render(label))
		}
	}
	return titleStyle.Render("📦 macinv — "+m.rep.Hostname) + "\n" + tabsRow.String()
}

func (m Model) browseView() string {
	rows := m.tabs[m.active].rows
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	lh := m.listHeight()
	end := m.top + lh
	if end > len(rows) {
		end = len(rows)
	}
	for i := m.top; i < end; i++ {
		mark := " "
		if rows[i].target != "" && m.marked[rows[i].target] {
			mark = markStyle.Render("✓")
		}
		cursor := "  "
		if i == m.sel && rows[i].target != "" {
			cursor = "▸ "
		}
		body := cursor + rows[i].text
		if i == m.sel {
			body = selStyle.Render(cursor + rows[i].text)
		}
		b.WriteString(mark + " " + body + "\n")
	}
	for i := end - m.top; i < lh; i++ {
		b.WriteString("\n") // 여백 채워 로그 패널 위치 고정
	}

	b.WriteString(m.logPanel())

	marked := len(m.selectedTargets())
	status := ""
	if m.running {
		status = warnStyle.Render(" · ▶ 실행 중")
	} else if len(m.queue) > 0 {
		status = warnStyle.Render(fmt.Sprintf(" · 대기 %d", len(m.queue)))
	}
	// 정렬 토글은 용량을 표시하는 앱 탭에서만 의미가 있어 그 탭에서만 안내한다.
	sortHint := ""
	if m.tabs[m.active].title == "앱" {
		order := "이름순"
		if m.sortBySize {
			order = "용량순"
		}
		sortHint = fmt.Sprintf(" · s 정렬(%s)", order)
	}
	help := fmt.Sprintf("  space 선택(%d) · d 삭제 · u 업데이트%s · r 재스캔 · ←/→ 탭 · ↑/↓ 이동 · q 종료%s", marked, sortHint, status)
	b.WriteString(footerStyle.Render(help))
	return b.String()
}

// logPanel은 하단 실행 로그 패널이다(내용 있을 때만).
func (m Model) logPanel() string {
	if len(m.log) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(dimStyle.Render(strings.Repeat("─", 40)+" 로그") + "\n")
	start := len(m.log) - logViewLines
	if start < 0 {
		start = 0
	}
	for _, l := range m.log[start:] {
		switch {
		case strings.HasPrefix(l, "✓"):
			b.WriteString("  " + okStyle.Render(l) + "\n")
		case strings.HasPrefix(l, "✗"):
			b.WriteString("  " + errStyle.Render(l) + "\n")
		default:
			b.WriteString("  " + dimStyle.Render(l) + "\n")
		}
	}
	return b.String()
}

func (m Model) planView() string {
	var b strings.Builder
	icon, verb := "🧹", "삭제"
	if m.plan.Icon != "" {
		icon = m.plan.Icon
	}
	if m.plan.Verb != "" {
		verb = m.plan.Verb
	}
	b.WriteString(titleStyle.Render(icon+" "+verb+" 계획: "+m.plan.Query) + "\n\n")

	togglePos := map[int]int{} // planLine 인덱스 → toggleIdx 순번
	for pos, idx := range m.toggleIdx {
		togglePos[idx] = pos
	}
	for i, l := range m.planLines {
		if l.Kind == "target" {
			b.WriteString("\n")
		}
		prefix := "  "
		text := l.Text
		if l.Toggleable {
			box := "[ ]"
			if m.lowOn[l.Path] {
				box = markStyle.Render("[x]")
			}
			cursor := "  "
			if togglePos[i] == m.toggleSel {
				cursor = "▸ "
			}
			prefix = cursor + box + " "
		}
		b.WriteString(prefix + text + "\n")
	}

	if len(m.toggleIdx) > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  [확신낮음] %d개 — ↑/↓ 이동, space로 켜야 삭제 대상에 포함(기본 제외).", len(m.toggleIdx))) + "\n")
	}
	// 회수 예상은 지금 켜진 토글 기준으로 다시 계산한다(체크박스를 켜면 즉시 늘어난다).
	if rl := clean.ReclaimLine(m.plan, m.lowOn); rl != "" {
		b.WriteString("\n  " + okStyle.Render(rl) + "\n")
	}
	// 경고 문구는 계획 종류에 맞춘다: 휴지통 경로가 있으면 삭제, 없으면(업데이트 등) 명령 실행.
	hasTrash := false
	for _, t := range m.plan.Targets {
		if len(t.TrashPaths) > 0 {
			hasTrash = true
			break
		}
	}
	if hasTrash {
		b.WriteString(warnStyle.Render("\n  ⚠️  휴지통으로 이동합니다(복구 가능).") + "\n")
	} else {
		b.WriteString(warnStyle.Render("\n  ⚠️  위 명령을 실행합니다.") + "\n")
	}
	foot := "  enter: 승인   esc: 취소"
	if m.needConfirm {
		foot = "  enter: 승인(GUI 앱 → y/N 재확인)   esc: 취소"
	}
	b.WriteString(footerStyle.Render(foot))
	return b.String()
}

func (m Model) confirmView() string {
	var apps []string
	for _, t := range m.plan.Targets {
		if t.Kind == clean.KindApp {
			apps = append(apps, t.Name)
		}
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚠️  GUI 앱 삭제 확인") + "\n\n")
	b.WriteString("  다음 앱을 휴지통으로 옮깁니다:\n")
	for _, a := range apps {
		b.WriteString("    • " + a + "\n")
	}
	b.WriteString(warnStyle.Render("\n  정말 진행할까요?") + "\n")
	b.WriteString(footerStyle.Render("  y: 삭제   n/esc: 취소"))
	return b.String()
}

// --- 탭 구성 (report → rows) ---

func buildTabs(rep *report.Report, dres *disk.Result, sortBySize bool) []tab {
	s := rep.Sections
	outdated := report.OutdatedRows(rep)
	return []tab{
		{"앱", appRows(s.Apps, sortBySize), len(s.Apps)},
		{"brew", brewRows(s.Brew), brewCount(s.Brew)},
		{"패키지", packageRows(s), packageCount(s)},
		{"런타임", runtimeRows(rep.Runtimes, rep.VersionManagers), installedRuntimeCount(rep.Runtimes)},
		{"업데이트", outdatedRowsFrom(outdated), len(outdated)},
		{tabDisk, diskRows(dres), diskCount(dres)},
		{"launchd", launchdRows(s.Launchd), len(s.Launchd)},
	}
}

// tabDisk는 디스크 탭 제목이다. 키 처리에서 이 탭만 다른 플래너를 쓰므로 상수로 둔다.
const tabDisk = "디스크"

// diskTargetPrefix는 디스크 행의 target 접두사다.
// 다른 탭의 target(앱/패키지 이름)과 이름이 겹쳐도 섞이지 않게 네임스페이스를 붙인다.
const diskTargetPrefix = "disk:"

func diskCount(dres *disk.Result) int {
	if dres == nil {
		return 0
	}
	return len(dres.Entries)
}

func diskRows(dres *disk.Result) []row {
	if dres == nil || len(dres.Entries) == 0 {
		return []row{{text: "(측정된 디스크 사용처 없음 — r로 재스캔)"}}
	}
	rows := make([]row, 0, len(dres.Entries)+3)
	for _, e := range dres.Entries {
		kb := e.SizeKB
		tag := ""
		if e.Orphan {
			tag = warnStyle.Render("  ⚠️ 고아 캐시")
		}
		rows = append(rows, row{
			text: fmt.Sprintf("%10s  %-14s %-32s %s%s",
				report.HumanSize(&kb), disk.TierLabel(e.Tier), trunc(e.Name, 32),
				dimStyle.Render(trunc(e.Tool, 12)), tag),
			target: diskTargetPrefix + e.ID,
		})
		// 와일드카드 위치는 앱별 내역을 한 줄 더 보여준다(어느 앱이 먹고 있는지가 판단 근거다).
		if line := disk.BreakdownLine(e.Breakdown, 5); line != "" {
			rows = append(rows, row{text: "            " + dimStyle.Render("↳ "+trunc(line, 70))})
		}
	}
	total, reclaim := dres.TotalKB, dres.ReclaimableKB
	rows = append(rows,
		row{text: dimStyle.Render(strings.Repeat("─", 40))},
		row{text: fmt.Sprintf("%10s  합계", report.HumanSize(&total))},
		row{text: fmt.Sprintf("%10s  %s", report.HumanSize(&reclaim),
			okStyle.Render("정리 시 회수 가능 (안전+빌드산출물)"))},
	)
	if len(dres.Deep) > 0 {
		deepKB := 0
		for _, d := range dres.Deep {
			deepKB += d.SizeKB
		}
		rows = append(rows, row{text: fmt.Sprintf("%10s  오래된 node_modules %d개 (macinv disk --deep)",
			report.HumanSize(&deepKB), len(dres.Deep))})
	}
	if dres.Snapshots != nil && len(dres.Snapshots.Names) > 0 {
		rows = append(rows, row{text: warnStyle.Render(fmt.Sprintf(
			"            APFS 로컬 스냅샷 %d개 — 지워도 여유 공간이 바로 안 늘 수 있습니다",
			len(dres.Snapshots.Names)))})
	}
	return rows
}

func brewCount(b *report.Brew) int {
	if b == nil {
		return 0
	}
	return len(b.Formulae) + len(b.Casks)
}

func packageCount(s report.Sections) int {
	n := len(s.Pipx) + len(s.Cargo) + len(s.GoBin)
	if s.Npm != nil {
		n += len(s.Npm.Packages)
	}
	return n
}

func installedRuntimeCount(runtimes []report.Runtime) int {
	n := 0
	for _, rt := range runtimes {
		if rt.Installed {
			n++
		}
	}
	return n
}

// sortAppsBySize는 앱을 실제 디스크 점유(번들+데이터) 내림차순으로 정렬한 사본을 돌려준다.
// 용량을 못 잰 앱(--no-sizes 등)은 뒤로 보내고 그들끼리는 원래 순서(이름순)를 지킨다.
// 순수 함수 — 원본 슬라이스를 건드리지 않는다(rep.Sections.Apps는 다른 곳에서도 쓴다).
func sortAppsBySize(apps []report.App) []report.App {
	out := make([]report.App, len(apps))
	copy(out, apps)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].TotalSizeKB(), out[j].TotalSizeKB()
		if a == nil || b == nil {
			return a != nil && b == nil // 측정된 것이 먼저
		}
		return *a > *b
	})
	return out
}

func appRows(apps []report.App, sortBySize bool) []row {
	if len(apps) == 0 {
		return []row{{text: "(없음)"}}
	}
	if sortBySize {
		apps = sortAppsBySize(apps)
	}
	rows := make([]row, 0, len(apps))
	for _, a := range apps {
		ver := "-"
		if a.Version != nil {
			ver = *a.Version
		}
		name := strings.TrimSuffix(a.Name, ".app")
		rows = append(rows, row{
			// 용량은 번들+데이터 합계 — 번들만 보면 Electron 앱의 수십 GB 캐시가 안 보인다.
			// 합계 옆에 내역(번들/데이터)을 같이 보여준다: 데이터 쪽이 크면 clean으로 줄일 여지가 있다는 신호다.
			text: fmt.Sprintf("%-28s %10s  %-26s %s",
				trunc(name, 28), report.HumanSize(a.TotalSizeKB()),
				dimStyle.Render(report.SizeBreakdown(a)), dimStyle.Render(ver)),
			target: name,
		})
	}
	return rows
}

func brewRows(b *report.Brew) []row {
	if b == nil {
		return []row{{text: "(brew 미설치)"}}
	}
	var rows []row
	rows = append(rows, row{text: warnStyle.Render(fmt.Sprintf("── formulae (%d) ──", len(b.Formulae)))})
	for _, f := range b.Formulae {
		tag := ""
		if f.InstalledOnRequest {
			tag = dimStyle.Render(" (직접)")
		}
		rows = append(rows, row{text: fmt.Sprintf("%-26s %s%s", trunc(f.Name, 26), strval(f.Version), tag), target: f.Name})
	}
	rows = append(rows, row{text: warnStyle.Render(fmt.Sprintf("── casks (%d) ──", len(b.Casks)))})
	for _, c := range b.Casks {
		rows = append(rows, row{text: fmt.Sprintf("%-26s %s", trunc(c.Name, 26), strval(c.Version)), target: c.Name})
	}
	return rows
}

func packageRows(s report.Sections) []row {
	var rows []row
	if s.Npm != nil && len(s.Npm.Packages) > 0 {
		rows = append(rows, row{text: warnStyle.Render("── npm 전역 ──")})
		for _, p := range s.Npm.Packages {
			mark := ""
			if p.Outdated {
				mark = warnStyle.Render("  ⬆")
			}
			rows = append(rows, row{text: fmt.Sprintf("%-30s %s%s", trunc(p.Name, 30), strval(p.Version), mark), target: p.Name})
		}
	}
	if len(s.Pipx) > 0 {
		rows = append(rows, row{text: warnStyle.Render("── pipx ──")})
		for _, p := range s.Pipx {
			rows = append(rows, row{text: fmt.Sprintf("%-30s %s", trunc(p.Name, 30), strval(p.Version)), target: p.Name})
		}
	}
	if len(s.Cargo) > 0 {
		rows = append(rows, row{text: warnStyle.Render("── cargo ──")})
		for _, p := range s.Cargo {
			rows = append(rows, row{text: fmt.Sprintf("%-30s %s", trunc(p.Name, 30), strval(p.Version)), target: p.Name})
		}
	}
	if len(s.GoBin) > 0 {
		rows = append(rows, row{text: warnStyle.Render("── ~/go/bin ──")})
		for _, g := range s.GoBin {
			rows = append(rows, row{text: trunc(g.Name, 30), target: g.Name})
		}
	}
	if len(rows) == 0 {
		return []row{{text: "(패키지 매니저 항목 없음)"}}
	}
	return rows
}

func runtimeRows(runtimes []report.Runtime, managers []report.VersionManager) []row {
	var rows []row
	for _, rt := range runtimes {
		if !rt.Installed {
			rows = append(rows, row{text: fmt.Sprintf("%-8s %s", rt.Name, dimStyle.Render("미설치"))})
			continue
		}
		ver := rt.Version
		if ver == "" {
			ver = "?"
		}
		mgr := ""
		if len(rt.Origins) > 0 {
			mgr = rt.Origins[0].Manager
		}
		conflict := ""
		if rt.Conflict {
			conflict = warnStyle.Render("  ⚠️ 여러 경로")
		}
		rows = append(rows, row{text: fmt.Sprintf("%-8s %-10s [%s]%s", rt.Name, ver, mgr, conflict)})
	}
	if len(managers) > 0 {
		rows = append(rows, row{text: warnStyle.Render("── 버전 매니저 ──")})
		for _, mgr := range managers {
			rows = append(rows, row{text: fmt.Sprintf("%-8s %s", mgr.Name, strings.Join(mgr.Versions, ", "))})
		}
	}
	if len(rows) == 0 {
		return []row{{text: "(없음)"}}
	}
	return rows
}

func outdatedRowsFrom(rr []report.OutdatedRow) []row {
	if len(rr) == 0 {
		return []row{{text: okStyle.Render("✅ 모든 패키지가 최신 상태입니다.")}}
	}
	rows := make([]row, 0, len(rr))
	for _, r := range rr {
		rows = append(rows, row{
			text:   fmt.Sprintf("[%-4s] %-28s %s → %s", r.Source, trunc(r.Name, 28), r.Current, warnStyle.Render(r.Latest)),
			target: r.Name, // u=업데이트 / d=삭제 대상
		})
	}
	return rows
}

func launchdRows(items []report.LaunchdItem) []row {
	if len(items) == 0 {
		return []row{{text: "(없음)"}}
	}
	rows := make([]row, 0, len(items))
	for _, it := range items {
		label := "-"
		if it.Label != nil {
			label = *it.Label
		}
		rows = append(rows, row{text: fmt.Sprintf("%-40s %s", trunc(label, 40), dimStyle.Render(it.Kind))})
	}
	return rows
}

func strval(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

func trunc(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n-1]) + "…"
	}
	return s
}

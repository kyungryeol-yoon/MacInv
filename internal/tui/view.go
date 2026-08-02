package tui

import (
	"fmt"
	"strings"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
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
	help := fmt.Sprintf("  space 선택(%d) · d 삭제 · u 업데이트 · r 재스캔 · ←/→ 탭 · ↑/↓ 이동 · q 종료%s", marked, status)
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

func buildTabs(rep *report.Report) []tab {
	s := rep.Sections
	outdated := report.OutdatedRows(rep)
	return []tab{
		{"앱", appRows(s.Apps), len(s.Apps)},
		{"brew", brewRows(s.Brew), brewCount(s.Brew)},
		{"패키지", packageRows(s), packageCount(s)},
		{"런타임", runtimeRows(rep.Runtimes, rep.VersionManagers), installedRuntimeCount(rep.Runtimes)},
		{"업데이트", outdatedRowsFrom(outdated), len(outdated)},
		{"launchd", launchdRows(s.Launchd), len(s.Launchd)},
	}
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

func appRows(apps []report.App) []row {
	if len(apps) == 0 {
		return []row{{text: "(없음)"}}
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
			text:   fmt.Sprintf("%-28s %-14s %s", trunc(name, 28), report.HumanSize(a.TotalSizeKB()), dimStyle.Render(ver)),
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

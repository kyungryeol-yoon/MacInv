// Package tui는 스캔 결과를 탐색하고 그 자리에서 삭제(Phase A)까지 하는 bubbletea 대화형 뷰다.
// 스캔·삭제 로직은 source/clean 패키지에 있고, 여기서는 선택/렌더링/흐름만 담당한다.
// 계획 렌더는 clean.PlanLines/RenderPlanText를 CLI와 공유한다(로직 복제 없음).
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
	"github.com/kyungryeol-yoon/macinv/internal/source"
)

// 모드.
const (
	modeBrowse   = iota // 목록 탐색
	modeBuilding        // 계획 작성 중(비동기)
	modePlan            // 계획 모달
	modeConfirm         // GUI 앱 삭제 y/N 추가 확인
)

const (
	logMaxLines  = 200 // 보관하는 로그 최대 줄
	logViewLines = 6   // 하단 패널에 보이는 줄
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("240"))
	markStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// RescanFunc는 지정한 소스만 다시 스캔해 report를 돌려준다(cmd가 주입).
type RescanFunc func(ctx context.Context, sources []string) *report.Report

// PersistFunc는 병합된 리포트를 캐시(~/.macinv/inventory.json)에 기록한다(cmd가 주입).
// TUI 화면과 디스크 캐시가 어긋나지 않게 재스캔 병합 직후 호출된다.
type PersistFunc func(rep *report.Report) error

// row는 탭 안의 한 줄이다. target이 비어있지 않으면 삭제 가능한 항목이다.
type row struct {
	text   string
	target string
}

type tab struct {
	title string
	rows  []row
	count int // 실제 항목 수(탭 라벨 표시용). "(없음)"/"최신 상태" 같은 안내 행은 세지 않는다.
}

type job struct {
	plan    *clean.Plan
	sources []string
}

// Model은 TUI 상태다.
type Model struct {
	ctx     context.Context
	run     *iexec.Runner
	rescan  RescanFunc
	persist PersistFunc
	rep     *report.Report
	events  chan tea.Msg

	tabs   []tab
	active int
	sel    int
	top    int
	w, h   int

	marked map[string]bool // 멀티선택된 target

	sortBySize bool // 앱 탭 정렬: false=이름순(기본), true=용량순. s 키로 토글.

	mode int

	// 계획 모달 상태
	plan        *clean.Plan
	planLines   []clean.PlanLine
	toggleIdx   []int // planLines 중 확신낮음(토글 가능) 줄 인덱스
	toggleSel   int   // toggleIdx 내 커서
	lowOn       map[string]bool
	needConfirm bool
	buildingVerb string // modeBuilding 동안 표시할 동사("삭제"/"업데이트")

	// 실행/큐/로그
	running bool
	queue   []job
	log     []string
}

// New는 수집된 리포트와 부분 재스캔/캐시 기록 함수로 TUI 모델을 만든다.
func New(ctx context.Context, run *iexec.Runner, rep *report.Report, rescan RescanFunc, persist PersistFunc) Model {
	return Model{
		ctx: ctx, run: run, rescan: rescan, persist: persist, rep: rep,
		events: make(chan tea.Msg, 128),
		tabs:   buildTabs(rep, false),
		marked: map[string]bool{},
	}
}

// Run은 TUI 프로그램을 실행한다(대체 화면 사용).
func Run(ctx context.Context, run *iexec.Runner, rep *report.Report, rescan RescanFunc, persist PersistFunc) error {
	_, err := tea.NewProgram(New(ctx, run, rep, rescan, persist), tea.WithAltScreen()).Run()
	return err
}

// Init은 이벤트 스트림 수신 루프를 시작한다.
func (m Model) Init() tea.Cmd { return waitEvent(m.events) }

// --- 메시지 ---

type planMsg struct{ plan *clean.Plan }
type execLineMsg struct{ line string }
type jobDoneMsg struct {
	sources []string
	failed  int // 실패한 단계(명령/휴지통) 수 — 로그 스크롤로 ✗를 놓쳐도 요약으로 보이게
}
type rescannedMsg struct {
	rep     *report.Report
	sources []string
}

// waitEvent는 이벤트 채널에서 한 건을 받아 메시지로 돌려준다(스트리밍 루프).
func waitEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m Model) buildPlanCmd(names []string) tea.Cmd {
	return func() tea.Msg {
		return planMsg{plan: clean.BuildCombinedPlan(m.ctx, m.run, names)}
	}
}

func (m Model) buildUpgradePlanCmd(names []string) tea.Cmd {
	return func() tea.Msg {
		return planMsg{plan: clean.BuildCombinedUpgradePlan(m.ctx, m.run, names)}
	}
}

// runJobCmd는 goroutine에서 계획을 실행하며 각 줄을 이벤트 채널로 스트리밍한다.
func (m Model) runJobCmd(j job) tea.Cmd {
	ch := m.events
	ctx := m.ctx
	run := m.run
	return func() tea.Msg {
		failed := 0
		clean.Execute(ctx, run, j.plan,
			func(cmdline string, err error) {
				if err != nil {
					failed++
					ch <- execLineMsg{"✗ " + cmdline + " — " + err.Error()}
				} else {
					ch <- execLineMsg{"✓ " + cmdline}
				}
			},
			func(res clean.TrashResult) {
				if res.Err != nil {
					failed++
					ch <- execLineMsg{"✗ 휴지통 실패: " + res.Path + " — " + res.Err.Error()}
				} else {
					ch <- execLineMsg{"✓ 휴지통으로 이동: " + res.Path}
				}
			},
		)
		ch <- jobDoneMsg{sources: j.sources, failed: failed}
		return nil
	}
}

func (m Model) rescanCmd(sources []string) tea.Cmd {
	return func() tea.Msg {
		return rescannedMsg{rep: m.rescan(m.ctx, sources), sources: sources}
	}
}

// Update은 키/리사이즈/스트리밍 이벤트를 처리한다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case planMsg:
		if msg.plan == nil || !msg.plan.HasContent() {
			verb := "삭제"
			if msg.plan != nil && msg.plan.Verb != "" {
				verb = msg.plan.Verb
			}
			m.appendLog(verb + "할 대상을 찾지 못했습니다.")
			m.mode = modeBrowse
			return m, nil
		}
		m.plan = msg.plan
		m.planLines = clean.PlanLines(msg.plan)
		m.lowOn = map[string]bool{}
		m.toggleIdx = m.toggleIdx[:0]
		for i, l := range m.planLines {
			if l.Toggleable {
				m.toggleIdx = append(m.toggleIdx, i)
			}
		}
		m.toggleSel = 0
		m.needConfirm = planHasApp(msg.plan)
		m.mode = modePlan
		return m, nil

	case execLineMsg:
		m.appendLog(msg.line)
		return m, waitEvent(m.events) // 계속 수신

	case jobDoneMsg:
		m.running = false
		if msg.failed > 0 {
			m.appendLog(fmt.Sprintf("── 작업 완료 (실패 %d건 — 위 ✗ 참조) ──", msg.failed))
		} else {
			m.appendLog("── 작업 완료 (모두 성공) ──")
		}
		cmds := []tea.Cmd{waitEvent(m.events)}
		if len(msg.sources) > 0 {
			cmds = append(cmds, m.rescanCmd(msg.sources))
		}
		if next, ok := m.startNext(); ok {
			cmds = append(cmds, next)
		}
		return m, tea.Batch(cmds...)

	case rescannedMsg:
		m.mergeRescan(msg.rep, msg.sources)
		m.clampScroll()
		// 화면과 디스크 캐시 동기화. 동기 호출인 이유: m.rep은 Update 루프에서만 변형되므로
		// 여기서 바로 직렬화해야 다음 병합과의 경합이 없다(리포트가 작아 지연은 무시 가능).
		if m.persist != nil {
			if err := m.persist(m.rep); err != nil {
				m.appendLog("캐시 저장 실패: " + err.Error())
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeBuilding:
		return m, nil // 계획 작성 중엔 대기

	case modePlan:
		return m.handlePlanKey(key)

	case modeConfirm:
		switch key {
		case "y", "Y":
			m.mode = modeBrowse
			cmd := m.enqueueAndMaybeStart()
			return m, cmd
		default: // n/N/esc/그외 → 취소
			m.mode = modeBrowse
		}
		return m, nil
	}

	// modeBrowse
	switch key {
	case "q":
		return m, tea.Quit
	case "esc": // 스펙: esc=취소 — 대기 큐가 있으면 큐만 비운다(실행 중 명령은 강제 종료 불가).
		if len(m.queue) > 0 {
			n := len(m.queue)
			m.queue = nil
			m.appendLog(fmt.Sprintf("대기 큐 %d건 취소됨 (실행 중 작업은 계속 진행)", n))
			return m, nil
		}
		return m, tea.Quit
	case "right", "tab", "l":
		m.active = (m.active + 1) % len(m.tabs)
		m.sel, m.top = 0, 0
	case "left", "shift+tab", "h":
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		m.sel, m.top = 0, 0
	case "down", "j":
		m.moveSel(1)
	case "up", "k":
		m.moveSel(-1)
	case " ":
		if r := m.currentRow(); r.target != "" {
			m.marked[r.target] = !m.marked[r.target]
		}
	case "d", "enter":
		if names := m.selectedTargets(); len(names) > 0 {
			m.mode = modeBuilding
			m.buildingVerb = "삭제"
			return m, m.buildPlanCmd(names)
		}
	case "u": // 업데이트(brew/npm/mas 최신 버전으로)
		if names := m.selectedTargets(); len(names) > 0 {
			m.mode = modeBuilding
			m.buildingVerb = "업데이트"
			return m, m.buildUpgradePlanCmd(names)
		}
	case "s": // 앱 탭 정렬 토글(이름순 ↔ 용량순)
		m.sortBySize = !m.sortBySize
		m.tabs = buildTabs(m.rep, m.sortBySize)
		// 행 순서가 통째로 바뀌므로 커서를 맨 위로 되돌린다(marked는 target 기준이라 유지된다).
		m.sel, m.top = 0, 0
	case "r":
		m.appendLog("전체 재스캔 중…")
		return m, m.rescanCmd(nil)
	}
	return m, nil
}

func (m Model) handlePlanKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.mode = modeBrowse
	case "up", "k":
		if len(m.toggleIdx) > 0 && m.toggleSel > 0 {
			m.toggleSel--
		}
	case "down", "j":
		if len(m.toggleIdx) > 0 && m.toggleSel < len(m.toggleIdx)-1 {
			m.toggleSel++
		}
	case " ":
		if len(m.toggleIdx) > 0 {
			path := m.planLines[m.toggleIdx[m.toggleSel]].Path
			m.lowOn[path] = !m.lowOn[path]
		}
	case "enter":
		if m.needConfirm {
			m.mode = modeConfirm
			return m, nil
		}
		m.mode = modeBrowse
		cmd := m.enqueueAndMaybeStart()
		return m, cmd
	}
	return m, nil
}

// enqueueAndMaybeStart는 현재 모달 계획을 effective로 만들어 큐에 넣고, 미실행이면 시작한다.
func (m *Model) enqueueAndMaybeStart() tea.Cmd {
	eff := m.plan.WithSelectedLow(m.lowOn)
	j := job{plan: eff, sources: planSources(eff)}
	// 성공적으로 큐에 넣었으니 관련 마크 해제.
	for _, n := range m.selectedTargets() {
		delete(m.marked, n)
	}
	m.queue = append(m.queue, j)
	m.appendLog(fmt.Sprintf("대기열에 추가: %s", eff.Query))
	if cmd, ok := m.startNext(); ok {
		return cmd
	}
	return nil
}

// startNext는 실행 중이 아니고 큐가 있으면 다음 잡을 시작한다(동시 1개만).
func (m *Model) startNext() (tea.Cmd, bool) {
	if m.running || len(m.queue) == 0 {
		return nil, false
	}
	j := m.queue[0]
	m.queue = m.queue[1:]
	m.running = true
	m.appendLog("▶ 실행: " + j.plan.Query)
	return m.runJobCmd(j), true
}

func (m *Model) moveSel(delta int) {
	rows := m.tabs[m.active].rows
	if len(rows) == 0 {
		return
	}
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(rows) {
		m.sel = len(rows) - 1
	}
	m.clampScroll()
}

func (m *Model) clampScroll() {
	lh := m.listHeight()
	if m.sel < m.top {
		m.top = m.sel
	}
	if lh > 0 && m.sel >= m.top+lh {
		m.top = m.sel - lh + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m Model) currentRow() row {
	rows := m.tabs[m.active].rows
	if m.sel >= 0 && m.sel < len(rows) {
		return rows[m.sel]
	}
	return row{}
}

// selectedTargets는 마크된 target들(없으면 현재 행)을 돌려준다.
func (m Model) selectedTargets() []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range m.tabs {
		for _, r := range t.rows {
			if r.target != "" && m.marked[r.target] && !seen[r.target] {
				seen[r.target] = true
				out = append(out, r.target)
			}
		}
	}
	if len(out) == 0 {
		if r := m.currentRow(); r.target != "" {
			out = append(out, r.target)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Model) appendLog(s string) {
	m.log = append(m.log, s)
	if len(m.log) > logMaxLines {
		m.log = m.log[len(m.log)-logMaxLines:]
	}
}

// mergeRescan은 재스캔된 소스 섹션만 저장 report에 반영하고 탭을 다시 만든다.
func (m *Model) mergeRescan(fresh *report.Report, sources []string) {
	if fresh == nil {
		m.appendLog("재스캔 실패")
		return
	}
	names := sources
	if len(names) == 0 { // 전체 재스캔
		names = []string{source.NameApps, source.NameBrew, source.NameNpm, source.NamePipx, source.NameCargo, source.NameGoBin, source.NameMas}
	}
	for _, n := range names {
		switch n {
		case source.NameApps:
			m.rep.Sections.Apps = fresh.Sections.Apps
		case source.NameBrew:
			m.rep.Sections.Brew = fresh.Sections.Brew
		case source.NameNpm:
			m.rep.Sections.Npm = fresh.Sections.Npm
		case source.NamePipx:
			m.rep.Sections.Pipx = fresh.Sections.Pipx
		case source.NameCargo:
			m.rep.Sections.Cargo = fresh.Sections.Cargo
		case source.NameGoBin:
			m.rep.Sections.GoBin = fresh.Sections.GoBin
		case source.NameMas:
			m.rep.Sections.Mas = fresh.Sections.Mas
		}
	}
	m.tabs = buildTabs(m.rep, m.sortBySize)
	if m.sel >= len(m.tabs[m.active].rows) {
		m.sel = len(m.tabs[m.active].rows) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	m.appendLog("목록 갱신됨: " + strings.Join(names, ", "))
}

// planHasApp은 계획에 GUI 앱 타깃이 있는지 본다(추가 확인 필요 판단).
func planHasApp(p *clean.Plan) bool {
	for _, t := range p.Targets {
		if t.Kind == clean.KindApp {
			return true
		}
	}
	return false
}

// planSources는 계획이 영향을 주는 소스 이름 목록이다(부분 재스캔용).
func planSources(p *clean.Plan) []string {
	set := map[string]bool{}
	for _, t := range p.Targets {
		switch t.Kind {
		case clean.KindApp:
			set[source.NameApps] = true
		case clean.KindBrewFormula, clean.KindBrewCask:
			set[source.NameBrew] = true
		case clean.KindNpm:
			set[source.NameNpm] = true
		case clean.KindPipx:
			set[source.NamePipx] = true
		case clean.KindCargo:
			set[source.NameCargo] = true
		case clean.KindGoBin:
			set[source.NameGoBin] = true
		case clean.KindMas:
			set[source.NameMas] = true
		}
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

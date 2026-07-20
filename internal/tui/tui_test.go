package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

func sampleReport() *report.Report {
	return &report.Report{
		Hostname: "test-host",
		Sections: report.Sections{
			Apps: []report.App{
				{Name: "Godot", Version: report.Ptr("4.6.2")},
				{Name: "iTerm", Version: report.Ptr("3.6.11")},
			},
			Brew: &report.Brew{
				Formulae: []report.Formula{{Name: "git", Version: report.Ptr("2.53.0"), InstalledOnRequest: true, Outdated: true, LatestVersion: report.Ptr("2.55.0")}},
				Casks:    []report.Cask{{Name: "iterm2", Version: report.Ptr("3.5.6")}},
			},
			Npm: &report.Npm{Packages: []report.NpmPackage{
				{Name: "@google/gemini-cli", Version: report.Ptr("0.42.0"), Outdated: true, LatestVersion: report.Ptr("0.51.0")},
			}},
		},
		Runtimes: []report.Runtime{
			{Name: "go", Installed: true, Version: "1.26.5", Origins: []report.Origin{{Manager: "brew"}}},
		},
		VersionManagers: []report.VersionManager{{Name: "nvm", Versions: []string{"v22.22.2"}}},
	}
}

func noopRescan(_ context.Context, _ []string) *report.Report { return sampleReport() }
func noopPersist(_ *report.Report) error                      { return nil }

func newTestModel() Model {
	run := iexec.NewRunner(&iexec.Recorder{}, 30)
	m := New(context.Background(), run, sampleReport(), noopRescan, noopPersist)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return updated.(Model)
}

func TestBuildTabsRows(t *testing.T) {
	tabs := buildTabs(sampleReport())
	wantTitles := []string{"앱", "brew", "패키지", "런타임", "업데이트", "launchd"}
	if len(tabs) != len(wantTitles) {
		t.Fatalf("탭 개수 = %d, want %d", len(tabs), len(wantTitles))
	}
	for i, w := range wantTitles {
		if tabs[i].title != w {
			t.Errorf("tabs[%d].title = %q, want %q", i, tabs[i].title, w)
		}
	}
	for _, r := range tabs[0].rows {
		if r.target == "" {
			t.Errorf("앱 행에 target 없음: %q", r.text)
		}
	}
	var sawGemini bool
	for _, r := range tabs[2].rows {
		if r.target == "@google/gemini-cli" {
			sawGemini = true
		}
	}
	if !sawGemini {
		t.Errorf("패키지 탭에 @google/gemini-cli 없음")
	}

	// 탭 라벨 카운트는 실제 항목 수여야 한다(플레이스홀더 행 제외). sampleReport는
	// 앱 2개, brew formula 1 + cask 1, npm 1, 런타임 설치 1(go)이므로 그대로 대응.
	wantCounts := map[string]int{"앱": 2, "brew": 2, "패키지": 1, "런타임": 1}
	for _, tb := range tabs {
		if want, ok := wantCounts[tb.title]; ok && tb.count != want {
			t.Errorf("tabs[%s].count = %d, want %d", tb.title, tb.count, want)
		}
	}
}

// 회귀: 업데이트할 게 없으면 "✅ 모든 패키지가 최신 상태입니다" 안내 행이 rows에는 있어도
// 탭 라벨 카운트는 0이어야 한다(전에는 안내 행까지 세어 "업데이트 (1)"로 잘못 표시됐다).
func TestOutdatedTabCountExcludesPlaceholder(t *testing.T) {
	rep := sampleReport()
	// 모든 outdated 플래그를 끈다.
	rep.Sections.Brew.Formulae[0].Outdated = false
	rep.Sections.Npm.Packages[0].Outdated = false

	tabs := buildTabs(rep)
	var updateTab *tab
	for i := range tabs {
		if tabs[i].title == "업데이트" {
			updateTab = &tabs[i]
		}
	}
	if updateTab == nil {
		t.Fatal("업데이트 탭을 못 찾음")
	}
	if updateTab.count != 0 {
		t.Errorf("업데이트할 게 없는데 count = %d, want 0", updateTab.count)
	}
	if len(updateTab.rows) != 1 {
		t.Errorf("안내 행이 없어짐: rows = %v", updateTab.rows)
	}
}

func TestMultiSelectToggle(t *testing.T) {
	m := newTestModel()
	// 앱 탭 첫 행에서 space → 마크.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(Model)
	if !m.marked["Godot"] {
		t.Fatalf("space 후 Godot 마크 안 됨: %v", m.marked)
	}
	// 아래로 이동 후 또 space → 두 개 선택.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(Model)
	sel := m.selectedTargets()
	if len(sel) != 2 {
		t.Errorf("선택 개수 = %d, want 2 (%v)", len(sel), sel)
	}
}

// u=업데이트 키가 현재/선택 대상으로 계획 작성을 시작한다.
func TestUpgradeKeyStartsBuilding(t *testing.T) {
	m := newTestModel()
	// 업데이트 탭(tabs[4])으로 이동해 outdated 행 선택.
	for i := 0; i < 4; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(Model)
	}
	if m.currentRow().target == "" {
		t.Fatalf("업데이트 탭 첫 행에 target 없음(u 불가): %q", m.currentRow().text)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updated.(Model)
	if m.mode != modeBuilding {
		t.Errorf("u 후 mode=%d, want modeBuilding", m.mode)
	}
	if cmd == nil {
		t.Errorf("u 후 업데이트 계획 커맨드가 없음")
	}
	// 빌드 대기 화면이 '업데이트 계획 작성 중'이어야 한다(삭제 아님).
	if view := m.View(); !strings.Contains(view, "업데이트 계획 작성 중") {
		t.Errorf("u 빌드 화면이 업데이트 문구가 아님:\n%s", view)
	}
}

// 업데이트 계획 모달은 '휴지통' 문구가 아니라 '명령 실행' 문구를 보여야 한다.
func TestUpgradeModalNoTrashWarning(t *testing.T) {
	m := newTestModel()
	up := &clean.Plan{Query: "git", Verb: "업데이트", Icon: "⬆️", Targets: []clean.Target{{
		Kind: clean.KindBrewFormula, Name: "git", Commands: []string{"brew upgrade git"},
	}}}
	updated, _ := m.Update(planMsg{plan: up})
	m = updated.(Model)
	view := m.planView()
	if strings.Contains(view, "휴지통으로 이동") {
		t.Errorf("업데이트 모달에 휴지통 문구가 있음:\n%s", view)
	}
	if !strings.Contains(view, "위 명령을 실행") {
		t.Errorf("업데이트 모달에 '명령 실행' 문구가 없음:\n%s", view)
	}
	if !strings.Contains(view, "업데이트 계획") {
		t.Errorf("업데이트 제목이 없음")
	}
	// GUI 앱이 없으므로 enter는 confirm 없이 바로 실행 경로.
	if m.needConfirm {
		t.Errorf("업데이트인데 needConfirm=true")
	}
}

func TestNavigationClamp(t *testing.T) {
	m := newTestModel()
	for i := 0; i < 5; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.sel != len(m.tabs[0].rows)-1 {
		t.Errorf("끝에서 clamp 실패: sel=%d", m.sel)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.active != 1 || m.sel != 0 {
		t.Errorf("탭 전환 후 active=%d sel=%d, want 1,0", m.active, m.sel)
	}
}

// 계획 모달: 확신낮음 체크박스 기본 해제, WithSelectedLow가 반영.
func TestPlanModalLowConfidenceDefaultOff(t *testing.T) {
	m := newTestModel()
	m.mode = modePlan
	m.plan = &clean.Plan{Query: "TestApp", Targets: []clean.Target{{
		Kind: clean.KindApp, Name: "TestApp.app",
		TrashPaths: []clean.PathMatch{
			{Path: "/x/high", Confidence: clean.ConfHigh, Reason: "확실"},
			{Path: "/x/low", Confidence: clean.ConfLow, Reason: "이름 기반"},
		},
	}}}
	// planMsg를 통해 모달 상태 초기화.
	updated, _ := m.Update(planMsg{plan: m.plan})
	m = updated.(Model)
	if len(m.toggleIdx) != 1 {
		t.Fatalf("토글 가능 줄 = %d, want 1", len(m.toggleIdx))
	}
	if m.lowOn["/x/low"] {
		t.Errorf("확신낮음이 기본 켜져 있음")
	}
	// 기본 effective엔 low 제외.
	eff := m.plan.WithSelectedLow(m.lowOn)
	if pathInPlan(eff, "/x/low") {
		t.Errorf("기본 effective에 확신낮음 포함됨")
	}
	// space로 켠 뒤엔 포함.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(Model)
	eff = m.plan.WithSelectedLow(m.lowOn)
	if !pathInPlan(eff, "/x/low") {
		t.Errorf("space로 켰는데 effective에 없음")
	}
	// GUI 앱이므로 enter → confirm 단계.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeConfirm {
		t.Errorf("GUI 앱 enter 후 mode=%d, want modeConfirm", m.mode)
	}
}

// 스트리밍: execLineMsg가 로그에 쌓이고, jobDoneMsg 후 running=false.
func TestStreamingLog(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(execLineMsg{"✓ 휴지통으로 이동: /x"})
	m = updated.(Model)
	if len(m.log) == 0 || !strings.Contains(m.log[len(m.log)-1], "/x") {
		t.Errorf("로그에 스트리밍 줄 없음: %v", m.log)
	}
	m.running = true
	updated, _ = m.Update(jobDoneMsg{sources: []string{"apps"}})
	m = updated.(Model)
	if m.running {
		t.Errorf("jobDoneMsg 후에도 running=true")
	}
}

// 큐: 실행 중이면 다음 잡은 큐에 남고 동시 1개만.
func TestQueueSingleFlight(t *testing.T) {
	m := newTestModel()
	m.plan = &clean.Plan{Query: "a", Targets: []clean.Target{{Kind: clean.KindBrewFormula, Name: "a", Commands: []string{"brew uninstall a"}}}}
	m.lowOn = map[string]bool{}
	m.running = true // 이미 실행 중이라고 가정
	m.enqueueAndMaybeStart()
	if len(m.queue) != 1 {
		t.Errorf("실행 중인데 큐에 안 쌓임: %d", len(m.queue))
	}
}

// 부분 재스캔: 잡 완료 후 해당 소스 탭 rows가 새 report로 교체된다.
func TestPartialRescanReplacesRows(t *testing.T) {
	m := newTestModel()
	// Godot이 삭제된 것처럼 apps에서 빠진 report.
	fresh := sampleReport()
	fresh.Sections.Apps = []report.App{{Name: "iTerm", Version: report.Ptr("3.6.11")}}

	updated, _ := m.Update(rescannedMsg{rep: fresh, sources: []string{"apps"}})
	m = updated.(Model)

	for _, r := range m.tabs[0].rows {
		if r.target == "Godot" {
			t.Errorf("재스캔 후에도 Godot이 남아있음")
		}
	}
	// 재스캔하지 않은 brew 탭은 그대로.
	var sawGit bool
	for _, r := range m.tabs[1].rows {
		if r.target == "git" {
			sawGit = true
		}
	}
	if !sawGit {
		t.Errorf("brew 탭이 잘못 바뀜 (git 사라짐)")
	}
}

// 재스캔 병합 직후 캐시 persist가 호출되어 화면과 디스크가 동기화된다.
func TestPersistCalledAfterRescan(t *testing.T) {
	called := 0
	run := iexec.NewRunner(&iexec.Recorder{}, 30)
	m := New(context.Background(), run, sampleReport(), noopRescan,
		func(_ *report.Report) error { called++; return nil })
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	updated, _ = m.Update(rescannedMsg{rep: sampleReport(), sources: []string{"apps"}})
	m = updated.(Model)
	if called != 1 {
		t.Errorf("재스캔 후 persist 호출 수 = %d, want 1", called)
	}
	// persist 실패는 로그로만 남는다.
	m.persist = func(_ *report.Report) error { return context.DeadlineExceeded }
	updated, _ = m.Update(rescannedMsg{rep: sampleReport(), sources: []string{"apps"}})
	m = updated.(Model)
	if !strings.Contains(strings.Join(m.log, "\n"), "캐시 저장 실패") {
		t.Errorf("persist 실패가 로그에 없음: %v", m.log)
	}
}

// 잡 완료 요약: 실패가 있으면 건수가 로그에 표시된다.
func TestJobDoneFailureSummary(t *testing.T) {
	m := newTestModel()
	m.running = true
	updated, _ := m.Update(jobDoneMsg{sources: nil, failed: 2})
	m = updated.(Model)
	if !strings.Contains(strings.Join(m.log, "\n"), "실패 2건") {
		t.Errorf("실패 요약이 로그에 없음: %v", m.log)
	}
	m.running = true
	updated, _ = m.Update(jobDoneMsg{sources: nil, failed: 0})
	m = updated.(Model)
	if !strings.Contains(strings.Join(m.log, "\n"), "모두 성공") {
		t.Errorf("성공 요약이 로그에 없음: %v", m.log)
	}
}

// esc는 대기 큐가 있으면 큐만 취소하고 앱을 종료하지 않는다(스펙 #1/#4).
func TestEscCancelsQueueNotQuit(t *testing.T) {
	m := newTestModel()
	m.queue = []job{{plan: &clean.Plan{Query: "pending"}}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil {
		t.Errorf("큐가 있는데 esc가 종료 커맨드를 반환")
	}
	if len(m.queue) != 0 {
		t.Errorf("esc 후에도 큐가 남음: %d", len(m.queue))
	}
	// 큐가 없으면 esc = 종료.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Errorf("큐 없는 esc가 종료하지 않음")
	}
}

func pathInPlan(p *clean.Plan, path string) bool {
	for _, t := range p.Targets {
		for _, pm := range t.TrashPaths {
			if pm.Path == path {
				return true
			}
		}
	}
	return false
}

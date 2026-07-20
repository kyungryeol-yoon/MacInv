package clean

import (
	"strings"
	"testing"
)

func samplePlan() *Plan {
	return &Plan{Query: "TestApp", Targets: []Target{{
		Kind:     KindApp,
		Name:     "TestApp.app",
		BundleID: "com.test.app",
		Warnings: []string{"의존성 있음"},
		TrashPaths: []PathMatch{
			{Path: "/Users/x/Library/Caches/com.test.app", Confidence: ConfHigh, Reason: "캐시"},
			{Path: "/Users/x/Library/Application Support/TestApp", Confidence: ConfLow, Reason: "이름 기반"},
		},
		Excluded:    []Exclusion{{Path: "/shared", Reason: "타 앱 사용 중"}},
		Suggestions: []string{"brew autoremove"},
	}}}
}

func TestPlanLinesToggleable(t *testing.T) {
	lines := PlanLines(samplePlan())
	var lowToggle, highToggle int
	for _, l := range lines {
		if l.Kind == "path" && l.Confidence == ConfLow {
			if l.Toggleable {
				lowToggle++
			}
		}
		if l.Kind == "path" && l.Confidence == ConfHigh && l.Toggleable {
			highToggle++
		}
	}
	if lowToggle != 1 {
		t.Errorf("확신낮음 toggleable = %d, want 1", lowToggle)
	}
	if highToggle != 0 {
		t.Errorf("확실 경로가 toggleable로 표시됨")
	}
}

func TestRenderPlanTextContainsAll(t *testing.T) {
	out := RenderPlanText(samplePlan())
	for _, want := range []string{
		"삭제 계획: 'TestApp'",
		"TestApp.app",
		"Bundle ID: com.test.app",
		"의존성 있음",
		"[확실]",
		"[확신낮음]",
		"보존: /shared",
		"brew autoremove",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderPlanText에 %q 없음\n---\n%s", want, out)
		}
	}
}

func TestWithSelectedLow(t *testing.T) {
	p := samplePlan()
	lowPath := "/Users/x/Library/Application Support/TestApp"

	// 기본(nil): 확신낮음 제외.
	def := p.WithSelectedLow(nil)
	if planHasPath(def, lowPath) {
		t.Errorf("기본에 확신낮음 포함됨")
	}
	if !planHasPath(def, "/Users/x/Library/Caches/com.test.app") {
		t.Errorf("확실 경로가 빠짐")
	}
	// 선택 시 포함.
	on := p.WithSelectedLow(map[string]bool{lowPath: true})
	if !planHasPath(on, lowPath) {
		t.Errorf("선택했는데 확신낮음 빠짐")
	}
}

func TestRenderUpgradePlanTitle(t *testing.T) {
	p := &Plan{Query: "git", Verb: "업데이트", Icon: "⬆️", Targets: []Target{{
		Kind: KindBrewFormula, Name: "git", Commands: []string{"brew upgrade git"},
	}}}
	out := RenderPlanText(p)
	if !strings.Contains(out, "⬆️ 업데이트 계획: 'git'") {
		t.Errorf("업데이트 제목이 안 나옴\n%s", out)
	}
	if !strings.Contains(out, "brew upgrade git") {
		t.Errorf("업그레이드 명령이 안 나옴")
	}
	// 기본(Verb 미설정) Plan은 '삭제'로 렌더.
	if !strings.Contains(RenderPlanText(samplePlan()), "🧹 삭제 계획") {
		t.Errorf("기본 Plan이 삭제 제목이 아님")
	}
}

func planHasPath(p *Plan, path string) bool {
	for _, t := range p.Targets {
		for _, pm := range t.TrashPaths {
			if pm.Path == path {
				return true
			}
		}
	}
	return false
}

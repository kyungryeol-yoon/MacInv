package clean

import (
	"testing"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// npm 셀프 업데이트는 npx 경유 명령이어야 한다(자기 파일 삭제로 인한 MODULE_NOT_FOUND 방지).
func TestUpgradeNpmSelfUsesNpx(t *testing.T) {
	// BuildUpgradePlan은 시스템 조회를 하므로, 명령 생성 규칙만 직접 검증한다.
	tgt := npmUpgradeTarget("npm")
	if len(tgt.Argv) != 1 || ShellQuote(tgt.Argv[0]) != "npx -y npm@latest install -g npm@latest" {
		t.Errorf("npm 셀프 업데이트 명령 오류: %v", tgt.Argv)
	}
	if len(tgt.Warnings) == 0 {
		t.Errorf("npm 셀프 업데이트 경고가 없음")
	}

	other := npmUpgradeTarget("@google/gemini-cli")
	if len(other.Argv) != 1 || ShellQuote(other.Argv[0]) != "npm install -g @google/gemini-cli@latest" {
		t.Errorf("일반 npm 패키지 명령 오류: %v", other.Argv)
	}
}

// 업그레이드 타깃은 상향된 타임아웃(30분)을 가진다 — 대형 다운로드 대비, 삭제는 기본 유지.
func TestUpgradeTargetTimeout(t *testing.T) {
	if got := npmUpgradeTarget("npm").CmdTimeoutSec; got != upgradeTimeoutSec {
		t.Errorf("npm 업그레이드 타임아웃 = %d, want %d", got, upgradeTimeoutSec)
	}
	if upgradeTimeoutSec != 1800 {
		t.Errorf("upgradeTimeoutSec = %d, want 1800", upgradeTimeoutSec)
	}
}

// 런타임 메이저 점프는 경고해야 한다 — update --all이 아무 말 없이 node 25→26을 올리면 안 된다.
func TestRuntimeMajorJump(t *testing.T) {
	cases := []struct {
		name, current, latest string
		want                  bool
	}{
		{"node", "25.9.0_2", "26.5.1", true},
		{"node", "25.9.0_2", "25.10.0", false}, // 같은 메이저 = 안전한 패치
		{"python", "3.13.1", "3.14.0", true},   // 3.13→3.14가 python의 사이클
		{"python", "3.13.1", "3.13.9", false},  // 같은 사이클
		{"ruby", "3.3.0", "3.4.1", true},
		// go/rust는 호환성 약속이 있어 1.x 안에서는 경고하지 않는다.
		{"go", "1.26.5", "1.27.0", false},
		{"rust", "1.96.1", "1.97.1", false},
		// @가 붙은 고정 버전 포뮬러는 메이저가 고정돼 있어 대상이 아니다.
		{"node@22", "22.22.2", "22.23.2", false},
		// 런타임이 아닌 포뮬러는 경고하지 않는다(소음이 된다).
		{"hugo", "0.163.0", "0.164.0", false},
		{"ripgrep", "14.1.0", "15.0.0", false},
	}
	for _, c := range cases {
		if got := RuntimeMajorJump(c.name, c.current, c.latest); got != c.want {
			t.Errorf("RuntimeMajorJump(%q, %q, %q) = %v, want %v", c.name, c.current, c.latest, got, c.want)
		}
	}

	// 계획에 실제로 경고가 붙는지.
	plan := UpgradePlanFromOutdated([]report.OutdatedRow{
		{Source: "brew", Name: "node", Current: "25.9.0_2", Latest: "26.5.1"},
		{Source: "brew", Name: "hugo", Current: "0.163.0", Latest: "0.164.0"},
	})
	if len(plan.Targets[0].Warnings) == 0 {
		t.Error("node 메이저 점프에 경고가 없음")
	}
	if len(plan.Targets[1].Warnings) != 0 {
		t.Errorf("hugo에 불필요한 경고: %v", plan.Targets[1].Warnings)
	}
}

// --all은 "생각 없이 실행하는 명령"이라 프로젝트를 깨뜨릴 수 있는 건 들어가면 안 된다.
// 런타임 메이저는 목록에만 남기고 실행 대상에서 뺀다. 패치는 그대로 포함한다.
func TestSplitRuntimeMajors(t *testing.T) {
	rows := []report.OutdatedRow{
		{Source: "brew", Name: "node", Current: "25.9.0_2", Latest: "26.5.1"},    // 메이저 → 보류
		{Source: "brew", Name: "node@22", Current: "22.22.2", Latest: "22.23.2"}, // 고정 버전 → 실행
		{Source: "brew", Name: "go", Current: "1.26.5", Latest: "1.27.0"},        // go는 1.x 호환 → 실행
		{Source: "brew", Name: "hugo", Current: "0.163.0", Latest: "0.164.0"},    // 런타임 아님 → 실행
		{Source: "npm", Name: "eas-cli", Current: "1.0.0", Latest: "2.0.0"},      // brew 아님 → 실행
		{Source: "brew", Name: "python", Current: "3.13.1", Latest: "3.14.0"},    // 메이저 → 보류
	}
	auto, held := SplitRuntimeMajors(rows)

	heldNames := map[string]bool{}
	for _, r := range held {
		heldNames[r.Name] = true
	}
	if len(held) != 2 || !heldNames["node"] || !heldNames["python"] {
		t.Errorf("보류 목록 = %v, want [node python]", heldNames)
	}
	autoNames := map[string]bool{}
	for _, r := range auto {
		autoNames[r.Name] = true
	}
	for _, want := range []string{"node@22", "go", "hugo", "eas-cli"} {
		if !autoNames[want] {
			t.Errorf("%s가 일괄 실행 대상에서 빠짐: %v", want, autoNames)
		}
	}
	if autoNames["node"] || autoNames["python"] {
		t.Errorf("런타임 메이저가 일괄 실행 대상에 들어감 — --all이 프로젝트를 깨뜨릴 수 있다: %v", autoNames)
	}

	// 계획에도 들어가면 안 된다.
	plan := UpgradePlanFromOutdated(auto)
	for _, tgt := range plan.Targets {
		if tgt.Name == "node" || tgt.Name == "python" {
			t.Errorf("계획에 런타임 메이저 타깃이 있음: %s", tgt.Name)
		}
	}

	// 빈 입력에도 안전.
	a, h := SplitRuntimeMajors(nil)
	if len(a) != 0 || len(h) != 0 {
		t.Errorf("빈 입력인데 결과가 있음: %v %v", a, h)
	}
}

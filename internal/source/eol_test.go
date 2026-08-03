package source

import (
	"strings"
	"testing"
	"time"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// 사이클 자릿수가 제각각이라(3.9 / 26 / 1.26) 가장 긴 접두사 일치로 골라야 한다.
func TestMatchCycle(t *testing.T) {
	python := []eolCycle{{Cycle: "3.14"}, {Cycle: "3.13"}, {Cycle: "3.9"}, {Cycle: "3.1"}}
	node := []eolCycle{{Cycle: "26"}, {Cycle: "25"}, {Cycle: "24"}, {Cycle: "2"}}
	golang := []eolCycle{{Cycle: "1.26"}, {Cycle: "1.25"}, {Cycle: "1.2"}}

	cases := []struct {
		name    string
		version string
		cycles  []eolCycle
		want    string
		found   bool
	}{
		{"python 패치", "3.9.6", python, "3.9", true},
		{"python 사이클 자체", "3.13", python, "3.13", true},
		{"node v 접두사", "v25.9.0", node, "25", true},
		{"java 메이저만", "openjdk 17.0.9", []eolCycle{{Cycle: "17"}, {Cycle: "21"}}, "17", true},
		// "1.2"가 "1.26.5"를 잘못 집으면 안 된다 — 가장 긴 일치가 이긴다.
		{"go 긴 접두사 우선", "go1.26.5", golang, "1.26", true},
		{"node 한 자리에 안 걸림", "25.9.0", node, "25", true},
		{"없는 사이클", "9.9.9", python, "", false},
		{"버전 없음", "", python, "", false},
	}
	for _, c := range cases {
		got, ok := MatchCycle(c.version, c.cycles)
		if ok != c.found || got.Cycle != c.want {
			t.Errorf("%s: MatchCycle(%q) = (%q, %v), want (%q, %v)",
				c.name, c.version, got.Cycle, ok, c.want, c.found)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"3.9.6":          "3.9.6",
		"v25.9.0":        "25.9.0",
		"go1.26.5":       "1.26.5",
		"openjdk 17.0.9": "17.0.9",
		"17":             "17",
		"26.0.2+10":      "26.0.2",
		"":               "",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEOLDateStatus(t *testing.T) {
	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		raw        any
		wantDate   string
		wantStatus string
	}{
		{"2025-10-31", "2025-10-31", EOLPassed}, // 이미 지남
		{"2026-11-30", "2026-11-30", EOLSoon},   // 1년 이내
		{"2029-10-31", "2029-10-31", EOLSupported},
		{false, "", EOLRolling}, // 종료일 미정(go/rust)
		{true, "", EOLPassed},
		{"이상한값", "", EOLUnknown},
		{nil, "", EOLUnknown},
	}
	for _, c := range cases {
		d, st := eolDateStatus(c.raw, now)
		if d != c.wantDate || st != c.wantStatus {
			t.Errorf("eolDateStatus(%v) = (%q, %q), want (%q, %q)", c.raw, d, st, c.wantDate, c.wantStatus)
		}
	}
}

// 메이저와 패치를 반드시 구분해야 한다 — 메이저는 프로젝트를 깨뜨린다.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name                                 string
		installed, latestInCycle, newest, my string
		want                                 string
	}{
		{"같은 사이클 최신", "1.26.5", "1.26.5", "1.26", "1.26", UpdateCurrent},
		{"같은 사이클 패치 있음", "1.26.1", "1.26.5", "1.26", "1.26", UpdatePatch},
		{"더 최신 메이저 존재", "3.9.6", "3.9.24", "3.14", "3.9", UpdateMajor},
		{"메이저가 우선(패치도 있어도)", "25.1.0", "25.9.0", "26", "25", UpdateMajor},
		{"설치 버전 모름", "", "1.26.5", "1.26", "1.26", UpdateUnknown},
		{"최신 정보 없음", "1.26.5", "", "1.26", "1.26", UpdateUnknown},
	}
	for _, c := range cases {
		if got := compareVersions(c.installed, c.latestInCycle, c.newest, c.my); got != c.want {
			t.Errorf("%s: = %q, want %q", c.name, got, c.want)
		}
	}
}

// 관리 주체별로 실제로 쓸 명령이 나와야 한다. 모르면 아무 말도 하지 않는다.
func TestSuggestUpgradeCommand(t *testing.T) {
	cases := []struct {
		name string
		ctx  SuggestCtx
		want string
	}{
		{"brew", SuggestCtx{Runtime: "go", Manager: MgrBrew}, "brew upgrade go"},
		{"brew java는 openjdk", SuggestCtx{Runtime: "java", Manager: MgrBrew}, "brew upgrade openjdk"},
		{"rustup", SuggestCtx{Runtime: "rust", Manager: MgrRustup}, "rustup update"},
		{"mise", SuggestCtx{Runtime: "python", Manager: MgrMise, LatestCycle: "3.14"}, "mise use -g python@3.14"},
		{"pyenv", SuggestCtx{Runtime: "python", Manager: MgrPyenv, LatestCycle: "3.14"},
			"pyenv install 3.14 && pyenv global 3.14"},
		{"사이클 모르면 latest", SuggestCtx{Runtime: "node", Manager: MgrMise}, "mise use -g node@latest"},
		{"관리 주체 모름 → 침묵", SuggestCtx{Runtime: "go", Manager: "manual"}, ""},
	}
	for _, c := range cases {
		if got := SuggestUpgradeCommand(c.ctx); got != c.want {
			t.Errorf("%s: = %q, want %q", c.name, got, c.want)
		}
	}

	// 시스템 런타임은 제자리 업그레이드가 안 되므로 "따로 설치"를 권한다.
	sys := SuggestUpgradeCommand(SuggestCtx{Runtime: "python", Manager: MgrSystem, LatestCycle: "3.14"})
	if sys == "" || !strings.Contains(sys, "brew install python") {
		t.Errorf("시스템 런타임 권장 명령 = %q", sys)
	}
	withMise := SuggestUpgradeCommand(SuggestCtx{Runtime: "python", Manager: MgrSystem, LatestCycle: "3.14", HasMise: true})
	if !strings.Contains(withMise, "mise use -g python@3.14") {
		t.Errorf("mise가 있으면 그걸 권해야 함: %q", withMise)
	}
}

// applyEOL이 사이클 정보를 런타임에 옮기는 전 과정(순수).
func TestApplyEOL(t *testing.T) {
	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	cycles := []eolCycle{
		{Cycle: "3.14", Latest: "3.14.6", EOL: "2030-10-31"},
		{Cycle: "3.9", Latest: "3.9.24", EOL: "2025-10-31"},
	}
	rt := report.Runtime{Name: "python", Installed: true, Version: "3.9.6"}
	applyEOL(&rt, cycles, now)

	if rt.EOLCycle != "3.9" || rt.LatestVersion != "3.9.24" || rt.LatestCycle != "3.14" {
		t.Errorf("사이클 매칭 실패: %+v", rt)
	}
	if rt.EOLStatus != EOLPassed || rt.EOLDate != "2025-10-31" {
		t.Errorf("EOL 상태 오류: %s %s", rt.EOLStatus, rt.EOLDate)
	}
	if rt.UpdateStatus != UpdateMajor {
		t.Errorf("업데이트 상태 = %s, want major", rt.UpdateStatus)
	}

	// 사이클을 못 찾으면 아무것도 채우지 않는다(잘못된 정보보다 없는 게 낫다).
	unknown := report.Runtime{Name: "python", Installed: true, Version: "9.9.9"}
	applyEOL(&unknown, cycles, now)
	if unknown.EOLStatus != "" || unknown.LatestVersion != "" {
		t.Errorf("모르는 버전인데 값이 채워짐: %+v", unknown)
	}
}

// 오프라인 + 캐시 없음이면 조용히 생략하고, 설치 현황은 그대로 남아야 한다.
func TestEnrichEOLOfflineIsSilent(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 캐시 없는 홈
	runtimes := []report.Runtime{
		{Name: "python", Installed: true, Version: "3.9.6",
			Origins: []report.Origin{{Manager: MgrSystem}}},
		{Name: "rust", Installed: false},
	}
	EnrichEOL(t.Context(), runtimes, nil, true, time.Now())

	if runtimes[0].Version != "3.9.6" || !runtimes[0].Installed {
		t.Error("설치 현황이 훼손됨")
	}
	if runtimes[0].EOLStatus != "" || runtimes[0].LatestVersion != "" {
		t.Errorf("오프라인인데 EOL 정보가 생김: %+v", runtimes[0])
	}
	// 권장 명령은 네트워크 없이도 나와야 한다(관리 주체만 알면 된다).
	if runtimes[0].SuggestedCommand == "" {
		t.Error("오프라인인데 권장 명령이 비었음")
	}
	if runtimes[0].Manager != MgrSystem || runtimes[0].InstalledVersion != "3.9.6" {
		t.Errorf("최상위 필드가 안 채워짐: %+v", runtimes[0])
	}
}

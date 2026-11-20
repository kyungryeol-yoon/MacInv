package clean

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyungryeol-yoon/MacInv/internal/paths"
)

// 이름 매칭: 정확 일치가 있으면 그것만, 없으면 부분일치 + exact=false(호출자가 경고 표시).
func TestMatchApps(t *testing.T) {
	entries := []appEntry{
		{name: "iTerm.app", path: "/Applications/iTerm.app"},
		{name: "iTerm Helper.app", path: "/Applications/iTerm Helper.app"},
		{name: "Godot.app", path: "/Applications/Godot.app"},
		{name: "notafile.txt", path: "/Applications/notafile.txt"},
	}
	// 정확 일치(대소문자 무시) → iTerm만, exact=true.
	got, exact := matchApps(entries, "iterm")
	if !exact || len(got) != 1 || got[0].name != "iTerm.app" {
		t.Errorf("정확 일치 실패: exact=%v got=%v", exact, got)
	}
	// 부분일치만 있는 질의 → 둘 다, exact=false.
	got, exact = matchApps(entries, "term")
	if exact || len(got) != 2 {
		t.Errorf("부분일치 실패: exact=%v got=%v", exact, got)
	}
	// .app 아닌 파일은 무시.
	got, _ = matchApps(entries, "notafile")
	if len(got) != 0 {
		t.Errorf(".app 아닌 항목이 매칭됨: %v", got)
	}
	// 아무것도 안 걸리면 빈 결과.
	if got, _ := matchApps(entries, "zzz"); len(got) != 0 {
		t.Errorf("무매칭인데 결과 있음: %v", got)
	}
}

// 접두사 glob이 삼킨 "다른 설치 앱 소유" 경로는 계획에서 빠지고 보존 사유가 남아야 한다.
// 핵심은 구분이다: .canary(다른 앱)는 제외, .helper(어느 앱도 소유 안 함)는 유지.
func TestExcludeOtherAppIDs(t *testing.T) {
	installed := []installedApp{
		{ID: "com.google.Chrome", Name: "Google Chrome"},
		{ID: "com.google.Chrome.canary", Name: "Google Chrome Canary"},
	}
	matches := []PathMatch{
		{Path: "/h/Library/Caches/com.google.Chrome", Confidence: ConfHigh},
		{Path: "/h/Library/Caches/com.google.Chrome.canary", Confidence: ConfHigh},
		{Path: "/h/Library/LaunchAgents/com.google.Chrome.helper.plist", Confidence: ConfHigh},
		{Path: "/h/Library/Application Support/com.google.Chrome.ShipIt", Confidence: ConfHigh},
	}
	kept, excluded := excludeOtherAppIDs(matches, "com.google.Chrome", installed)

	keptSet := map[string]bool{}
	for _, k := range kept {
		keptSet[k.Path] = true
	}
	for _, want := range []string{
		"/h/Library/Caches/com.google.Chrome",
		"/h/Library/LaunchAgents/com.google.Chrome.helper.plist",
		"/h/Library/Application Support/com.google.Chrome.ShipIt",
	} {
		if !keptSet[want] {
			t.Errorf("자기/파생 경로가 제외됨: %s", want)
		}
	}
	if keptSet["/h/Library/Caches/com.google.Chrome.canary"] {
		t.Error("타 앱(canary) 소유 경로가 계획에 남음 — [확실]이라 --apply 기본 삭제 대상이 된다")
	}
	if len(excluded) != 1 || !strings.Contains(excluded[0].Reason, "Google Chrome Canary") {
		t.Errorf("보존 사유가 잘못됨: %+v", excluded)
	}

	// 반대 방향: canary를 지울 때 canary 파생물이 Chrome 소유로 오판되면 안 된다(가장 긴 ID가 이긴다).
	kept, _ = excludeOtherAppIDs([]PathMatch{
		{Path: "/h/Library/Caches/com.google.Chrome.canary.helper"},
	}, "com.google.Chrome.canary", installed)
	if len(kept) != 1 {
		t.Error("canary 파생물이 Chrome 소유로 오판됨")
	}

	// Bundle ID를 모르면(빈 문자열) 아무것도 제외하지 않는다.
	kept, excluded = excludeOtherAppIDs(matches, "", installed)
	if len(kept) != len(matches) || len(excluded) != 0 {
		t.Error("selfID가 비었는데 경로가 제외됨")
	}
}

// 회귀: Chrome/Chrome Canary가 둘 다 설치된 상태에서 Chrome 계획에 canary 경로가 없어야 한다.
func TestLeftoversDoNotSwallowOtherApp(t *testing.T) {
	home := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	self := mk("Library/Caches/com.google.Chrome")
	canary := mk("Library/Caches/com.google.Chrome.canary")
	helper := mk("Library/LaunchAgents/com.google.Chrome.helper.plist")

	installed := func() []installedApp {
		return []installedApp{
			{ID: "com.google.Chrome", Name: "Google Chrome"},
			{ID: "com.google.Chrome.canary", Name: "Google Chrome Canary"},
		}
	}
	kept, excluded := filterOtherAppOwned(
		paths.LeftoverPaths(home, "com.google.Chrome", "Google Chrome"), "com.google.Chrome", installed)

	got := map[string]bool{}
	for _, k := range kept {
		got[k.Path] = true
	}
	if !got[self] || !got[helper] {
		t.Errorf("자기 경로/파생 헬퍼가 빠짐: %v", got)
	}
	if got[canary] {
		t.Error("Chrome 계획에 Chrome Canary 경로가 포함됨 (접두사 과매칭)")
	}
	if len(excluded) != 1 || excluded[0].Path != canary {
		t.Errorf("보존 목록이 잘못됨: %+v", excluded)
	}
}

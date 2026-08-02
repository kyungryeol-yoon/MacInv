package clean

import (
	"path/filepath"
	"strings"
	"testing"
)

// 쓰기 권한이 없는 경로는 TrashPaths에서 빠지고 "권한 필요"로 분리돼야 한다.
// 그대로 두면 휴지통 이동이 실패해 로그에 ✗만 남고, 회수 예상도 부풀려진다.
func TestSplitByWritability(t *testing.T) {
	matches := []PathMatch{
		{Path: "/Users/c/Library/Caches/com.example.app", Confidence: ConfHigh},
		{Path: "/Library/LaunchDaemons/com.example.app.plist", Confidence: ConfHigh},
		{Path: "/Library/Application Support/com.example.app", Confidence: ConfHigh},
	}
	// /Library 아래는 쓰기 불가로 흉내낸다.
	writable := func(p string) bool { return !strings.HasPrefix(filepath.Dir(p), "/Library") }

	ok, needAdmin := splitByWritability(matches, writable)
	if len(ok) != 1 || ok[0].Path != matches[0].Path {
		t.Errorf("휴지통 대상 = %v, want 사용자 홈 경로 1개", ok)
	}
	if len(needAdmin) != 2 {
		t.Errorf("권한 필요 = %v, want 2개", needAdmin)
	}
}

// markNeedsAdmin은 계획을 갈라 놓고 수동 처리 안내를 붙인다.
func TestMarkNeedsAdminRendersSection(t *testing.T) {
	plan := &Plan{Query: "x", Targets: []Target{{
		Kind: KindApp, Name: "X.app",
		TrashPaths: []PathMatch{
			// 존재하지 않는 부모 디렉토리 → access 실패 → 권한 필요로 분류된다.
			{Path: "/nonexistent-dir-for-test/com.example.app.plist", Confidence: ConfHigh, Reason: "LaunchDaemon (시스템)"},
		},
	}}}
	markNeedsAdmin(plan)
	if len(plan.Targets[0].TrashPaths) != 0 {
		t.Errorf("권한 없는 경로가 휴지통 대상에 남음: %v", plan.Targets[0].TrashPaths)
	}
	if len(plan.Targets[0].NeedsAdmin) != 1 {
		t.Fatalf("권한 필요 목록이 비었음")
	}
	text := RenderPlanText(plan)
	if !strings.Contains(text, "권한 필요(수동)") {
		t.Errorf("권한 필요 섹션이 렌더에 없음:\n%s", text)
	}
	if !strings.Contains(text, "sudo") {
		t.Errorf("수동 처리 안내가 없음:\n%s", text)
	}
}

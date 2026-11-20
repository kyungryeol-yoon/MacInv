package clean

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
)

func TestParseDuSizes(t *testing.T) {
	out := "12\t/Users/c/Library/Caches/com.example.app\n" +
		"2048\t/Users/c/Library/Application Support/My App With Spaces\n" +
		"쓰레기 줄\n" +
		"  \n"
	got := parseDuSizes(out)
	if got["/Users/c/Library/Caches/com.example.app"] != 12 {
		t.Errorf("숫자 파싱 실패: %v", got)
	}
	// 경로에 공백이 있어도 통째로 살아야 한다(공백 분리로 자르면 깨진다).
	if got["/Users/c/Library/Application Support/My App With Spaces"] != 2048 {
		t.Errorf("공백 포함 경로 파싱 실패: %v", got)
	}
	if len(got) != 2 {
		t.Errorf("잘못된 줄이 파싱됨: %v", got)
	}
}

// 회수 예상은 계획에 실제로 담긴 경로만, 그리고 확신낮음 토글 상태를 반영해 합산한다.
func TestReclaimKB(t *testing.T) {
	plan := &Plan{Query: "Foo", Targets: []Target{{
		Kind: KindApp, Name: "Foo.app",
		TrashPaths: []PathMatch{
			{Path: "/a", Confidence: ConfHigh, SizeKB: 1000},
			{Path: "/b", Confidence: ConfMedium, SizeKB: 500},
			{Path: "/c", Confidence: ConfLow, SizeKB: 8000},
			// 부모에 포함돼 0으로 표시된 경로는 더하지 않는다(이중 계산 방지).
			{Path: "/a/sub", Confidence: ConfHigh, SizeKB: 0},
		},
	}}}

	kb, ok := ReclaimKB(plan, nil)
	if !ok || kb != 1500 {
		t.Errorf("기본(확신낮음 제외) = %d, want 1500 (ok=%v)", kb, ok)
	}
	kb, _ = ReclaimKB(plan, map[string]bool{"/c": true})
	if kb != 9500 {
		t.Errorf("확신낮음 켠 뒤 = %d, want 9500", kb)
	}

	// 렌더 줄: 기본값 + 확신낮음 포함 시 상한을 함께 보여준다.
	line := ReclaimLine(plan, nil)
	if !strings.Contains(line, "회수 예상") || !strings.Contains(line, "확신낮음") {
		t.Errorf("ReclaimLine = %q", line)
	}

	// 측정값이 없으면(업데이트 계획 등) 줄을 만들지 않는다.
	if l := ReclaimLine(&Plan{Targets: []Target{{Argv: [][]string{{"brew", "upgrade", "foo"}}}}}, nil); l != "" {
		t.Errorf("측정값 없는데 회수 예상이 표시됨: %q", l)
	}
}

// 부모/자식이 함께 계획에 있으면 부모에만 용량이 실려야 한다(du 이중 계산 방지).
func TestMeasureTrashSizesParentOnly(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "blob"), make([]byte, 64*1024), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Targets: []Target{{TrashPaths: []PathMatch{
		{Path: child, Confidence: ConfHigh},
		{Path: dir, Confidence: ConfHigh},
	}}}}
	run := iexec.NewRunner(&iexec.Recorder{}, 30)
	MeasureTrashSizes(context.Background(), run, plan)

	got := map[string]int{}
	for _, pm := range plan.Targets[0].TrashPaths {
		got[pm.Path] = pm.SizeKB
	}
	if got[child] != 0 {
		t.Errorf("자식 경로에 용량이 실림(부모에 이미 포함됨): %d KB", got[child])
	}
	if got[dir] < 64 {
		t.Errorf("부모 경로 용량이 안 잡힘: %d KB", got[dir])
	}
	kb, ok := ReclaimKB(plan, nil)
	if !ok || kb != got[dir] {
		t.Errorf("회수 예상 = %d, want %d (이중 계산 없이 부모 값 그대로)", kb, got[dir])
	}
}

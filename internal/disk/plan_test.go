package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

func sampleResult() *Result {
	return &Result{Entries: []Entry{
		{ID: "npm-cache", Name: "npm 캐시", Tool: "npm", Tier: TierSafe, SizeKB: 2 * 1024 * 1024,
			Paths: []string{"/h/.npm/_cacache"}, Cleanup: [][]string{{"npm", "cache", "clean", "--force"}}},
		{ID: "ollama", Name: "Ollama 모델", Tool: "ollama", Tier: TierExpensive, SizeKB: 9 * 1024 * 1024,
			Paths: []string{"/h/.ollama/models"}, Note: "재다운로드 비쌈"},
		{ID: "orbstack", Name: "OrbStack", Tool: "orbstack", Tier: TierExpensive, SizeKB: 26 * 1024 * 1024,
			Paths: []string{"/h/gc/orbstack"}, Cleanup: [][]string{{"docker", "system", "prune", "-f"}}},
		{ID: "deriveddata", Name: "Xcode DerivedData", Tool: "xcodebuild", Tier: TierArtifact, SizeKB: 139 * 1024,
			Paths: []string{"/h/dd"}, Trash: true},
		{ID: "user-data", Name: "사용자 데이터", Tool: "x", Tier: TierNever, SizeKB: 100,
			Paths: []string{"/h/data"}, Cleanup: [][]string{{"rm", "-rf", "/h/data"}}},
		{ID: "gradle", Name: "Gradle 캐시", Tool: "gradle", Tier: TierSafe, SizeKB: 3 * 1024 * 1024,
			Paths: []string{"/h/.gradle"}, Note: "네이티브 명령 없음", Orphan: true},
	}}
}

// 기본 정리 계획은 safe+artifact만 담고, expensive/never/리포트전용은 뺀다.
func TestBuildCleanupPlanDefault(t *testing.T) {
	plan := BuildCleanupPlan(sampleResult(), nil, false)
	names := planNames(plan)

	if !contains(names, "npm 캐시") || !contains(names, "Xcode DerivedData") {
		t.Errorf("safe/artifact 항목이 빠짐: %v", names)
	}
	if contains(names, "Ollama 모델") || contains(names, "OrbStack") {
		t.Errorf("expensive가 기본 계획에 들어감: %v", names)
	}
	if contains(names, "사용자 데이터") {
		t.Errorf("never 등급이 계획에 들어감 — 절대 안 됨: %v", names)
	}
	// 네이티브 명령도 trash도 없는 위치는 계획에 담을 게 없다(리포트 전용).
	if contains(names, "Gradle 캐시") {
		t.Errorf("리포트 전용 위치가 계획에 들어감: %v", names)
	}

	// trash 위치는 명령이 아니라 휴지통 경로로 담긴다(rm 금지 규칙).
	for _, tgt := range plan.Targets {
		if strings.Contains(tgt.Name, "DerivedData") {
			if len(tgt.TrashPaths) != 1 || tgt.TrashPaths[0].Path != "/h/dd" {
				t.Errorf("DerivedData가 휴지통 경로로 안 담김: %+v", tgt)
			}
			if len(tgt.Argv) != 0 {
				t.Errorf("trash 위치에 명령이 붙음: %v", tgt.Argv)
			}
		}
	}
}

// --include-expensive를 줘야 모델/이미지가 포함된다(clean의 --include-low와 같은 패턴).
func TestBuildCleanupPlanIncludeExpensive(t *testing.T) {
	plan := BuildCleanupPlan(sampleResult(), nil, true)
	names := planNames(plan)
	if !contains(names, "OrbStack") {
		t.Errorf("--include-expensive인데 OrbStack이 없음: %v", names)
	}
	// 명령이 없는 expensive(Ollama)는 여전히 담을 게 없다 — 개별 선택 경로로 간다.
	if contains(names, "Ollama 모델") {
		t.Errorf("명령 없는 위치가 담김: %v", names)
	}
	if contains(names, "사용자 데이터") {
		t.Errorf("never 등급이 --include-expensive로 뚫림 — 절대 안 됨: %v", names)
	}
}

// TUI 멀티선택: 고른 ID만 담는다.
func TestBuildCleanupPlanSelected(t *testing.T) {
	plan := BuildCleanupPlan(sampleResult(), map[string]bool{"npm-cache": true}, false)
	if len(plan.Targets) != 1 || !strings.Contains(plan.Targets[0].Name, "npm") {
		t.Errorf("선택한 것만 담기지 않음: %v", planNames(plan))
	}
}

// 고아 캐시는 계획에도 경고로 남아야 한다(왜 지워도 되는지가 근거다).
func TestCleanupPlanKeepsOrphanWarning(t *testing.T) {
	res := &Result{Entries: []Entry{{
		ID: "gradle", Name: "Gradle 캐시", Tool: "gradle", Tier: TierSafe, Orphan: true,
		Paths: []string{"/h/.gradle"}, Cleanup: [][]string{{"true"}},
	}}}
	text := clean.RenderPlanText(BuildCleanupPlan(res, nil, false))
	if !strings.Contains(text, "고아 캐시") || !strings.Contains(text, "gradle") {
		t.Errorf("고아 경고가 계획에 없음:\n%s", text)
	}
}

func TestBuildOllamaPlan(t *testing.T) {
	models := []OllamaModel{
		{Name: "llama3:8b", SizeKB: 4 * 1024 * 1024},
		{Name: "qwen2.5-coder:14b", SizeKB: 9 * 1024 * 1024},
	}
	plan := BuildOllamaPlan(models, map[string]bool{"llama3:8b": true})
	if len(plan.Targets) != 1 {
		t.Fatalf("고른 모델만 담아야 함: %v", planNames(plan))
	}
	got := clean.ShellQuote(plan.Targets[0].Argv[0])
	if got != "ollama rm llama3:8b" {
		t.Errorf("명령 = %q", got)
	}
	// 아무것도 안 고르면 빈 계획.
	if BuildOllamaPlan(models, nil).HasContent() {
		t.Error("선택 없는데 계획에 내용 있음")
	}
}

// --deep은 6개월 이상 손대지 않은 node_modules만 집는다.
func TestScanDeepOnlyOldOnes(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	mk := func(rel string, age time.Duration) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "x"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		return p
	}
	old := mk("projects/old-app/node_modules", 400*24*time.Hour)
	mk("projects/active-app/node_modules", 10*24*time.Hour) // 최근 — 제외돼야 함
	mk(".hidden/tool/node_modules", 400*24*time.Hour)       // 숨김 디렉토리 — 들어가지 않는다

	run := iexec.NewRunner(&iexec.Recorder{}, 30)
	got := ScanDeep(context.Background(), run, home, now)

	if len(got) != 1 {
		t.Fatalf("결과 %d개, want 1 — %+v", len(got), got)
	}
	if got[0].Path != old {
		t.Errorf("찾은 경로 = %s, want %s", got[0].Path, old)
	}
	if got[0].AgeDays < 180 {
		t.Errorf("나이 계산 오류: %d일", got[0].AgeDays)
	}
	if got[0].SizeKB <= 0 {
		t.Errorf("용량이 측정되지 않음: %+v", got[0])
	}
}

func planNames(p *clean.Plan) []string {
	var out []string
	for _, t := range p.Targets {
		out = append(out, t.Name)
	}
	return out
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// 회수 예상은 명령이 지우는 몫까지 포함해야 한다.
// 휴지통 경로만 세면 요약의 "정리 시 회수 가능"과 계획 하단 숫자가 어긋난다(실측: 3.1 GB vs 139 MB).
func TestCleanupPlanReclaimIncludesCommandTargets(t *testing.T) {
	plan := BuildCleanupPlan(sampleResult(), nil, false)
	kb, ok := clean.ReclaimKB(plan, nil)
	if !ok {
		t.Fatal("회수 예상이 계산되지 않음")
	}
	// npm 캐시 2GB(명령) + DerivedData 139MB(휴지통 경로에 미리 실어둔 값).
	want := 2*1024*1024 + 139*1024
	if kb != want {
		t.Errorf("회수 예상 = %d KB, want %d", kb, want)
	}
	// 명령 대상만 있는 계획도 숫자가 나와야 한다(휴지통 경로 0건).
	only := BuildCleanupPlan(sampleResult(), map[string]bool{"npm-cache": true}, false)
	kb, ok = clean.ReclaimKB(only, nil)
	if !ok || kb != 2*1024*1024 {
		t.Errorf("명령 전용 계획 회수 예상 = %d KB (ok=%v), want %d", kb, ok, 2*1024*1024)
	}
	if clean.ReclaimLine(only, nil) == "" {
		t.Error("명령 전용 계획인데 회수 예상 줄이 비었음")
	}
}

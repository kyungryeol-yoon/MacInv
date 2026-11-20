package disk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyungryeol-yoon/MacInv/internal/clean"
)

func TestParseRunningApps(t *testing.T) {
	ps := "/Applications/Visual Studio Code.app/Contents/MacOS/Electron\n" +
		"/Applications/Claude.app/Contents/MacOS/Claude\n" +
		// 헬퍼가 앱 안에 중첩된 경우 — 바깥/안쪽 둘 다 담아야 놓치지 않는다.
		"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Helper.app/Contents/MacOS/x\n" +
		"/Library/Application Support/Logitech.localized/LogiOptionsPlus/logioptionsplus_agent.app/Contents/MacOS/y\n" +
		"/usr/sbin/syslogd\n" +
		"\n"
	got := ParseRunningApps(ps)
	set := map[string]bool{}
	for _, a := range got {
		set[a] = true
	}
	for _, want := range []string{
		"Visual Studio Code", "Claude", "Google Chrome", "Google Chrome Helper", "logioptionsplus_agent",
	} {
		if !set[want] {
			t.Errorf("앱 %q를 못 뽑음: %v", want, got)
		}
	}
	if set["syslogd"] {
		t.Error(".app이 아닌 프로세스가 앱으로 잡힘")
	}
	// 중복 제거.
	if len(got) != len(set) {
		t.Errorf("중복이 남음: %v", got)
	}
}

// 데이터 디렉토리 이름과 앱 이름은 자주 다르다. 애매하면 보호(=제외) 쪽으로 기운다.
func TestMatchesRunningApp(t *testing.T) {
	running := []string{"Visual Studio Code", "Claude", "logioptionsplus_agent", "Notion"}
	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"완전 일치", "Claude", true},
		{"완전 일치(대소문자 무시)", "notion", true},
		{"Code ↔ Visual Studio Code (단어 매칭)", "Code", true},
		{"LogiOptionsPlus ↔ logioptionsplus_agent", "LogiOptionsPlus", true},
		{"실행 중 아님", "Slack", false},
		{"실행 중 아님(비슷한 이름)", "Discord", false},
		// 부분 문자열이었다면 "Stu"가 "Visual Studio Code"에 걸렸을 것이다 — 단어 단위라 안 걸린다.
		{"부분 문자열은 매칭 안 함", "Stu", false},
		{"빈 이름", "", false},
	}
	for _, c := range cases {
		if got := MatchesRunningApp(c.dir, running); got != c.want {
			t.Errorf("%s: MatchesRunningApp(%q) = %v, want %v", c.name, c.dir, got, c.want)
		}
	}
	// 실행 중 목록이 비면(ps 실패 등) 아무것도 막지 않는다.
	if MatchesRunningApp("Claude", nil) {
		t.Error("실행 목록이 비었는데 매칭됨")
	}
}

func runningEntry() Entry {
	base := "/h/Library/Application Support/"
	return Entry{
		ID: "electron-cache", Name: "Electron 앱 캐시", Tier: TierSafe, Trash: true,
		Paths: []string{base + "Notion/Cache", base + "Slack/Cache", base + "Notion/GPUCache"},
		PathSizes: map[string]int{
			base + "Notion/Cache":    40 * 1024 * 1024,
			base + "Slack/Cache":     3 * 1024 * 1024,
			base + "Notion/GPUCache": 1 * 1024 * 1024,
		},
		SizeKB: 44 * 1024 * 1024,
		BlockedByRunning: map[string]string{
			base + "Notion/Cache":    "Notion",
			base + "Notion/GPUCache": "Notion",
		},
	}
}

// 실행 중인 앱의 캐시는 기본적으로 계획에서 빠지고 보존 사유가 남는다.
func TestBuildCleanupPlanExcludesRunning(t *testing.T) {
	res := &Result{Entries: []Entry{runningEntry()}}
	plan := BuildCleanupPlan(res, nil, false, false)
	tgt := plan.Targets[0]

	var trashed []string
	for _, pm := range tgt.TrashPaths {
		trashed = append(trashed, pm.Path)
	}
	if len(trashed) != 1 || !strings.HasSuffix(trashed[0], "Slack/Cache") {
		t.Errorf("휴지통 대상 = %v, want Slack/Cache만", trashed)
	}
	if len(tgt.Excluded) != 2 {
		t.Fatalf("보존 목록 = %v, want 2건", tgt.Excluded)
	}
	for _, e := range tgt.Excluded {
		if !strings.Contains(e.Reason, "실행 중(Notion)") || !strings.Contains(e.Reason, "종료") {
			t.Errorf("보존 사유가 부족함: %q", e.Reason)
		}
	}
	if len(tgt.Warnings) != 0 {
		t.Errorf("제외했는데 경고가 붙음: %v", tgt.Warnings)
	}
}

// --include-running이면 포함하되 위험을 분명히 알린다.
func TestBuildCleanupPlanIncludeRunning(t *testing.T) {
	res := &Result{Entries: []Entry{runningEntry()}}
	plan := BuildCleanupPlan(res, nil, false, true)
	tgt := plan.Targets[0]

	if len(tgt.TrashPaths) != 3 {
		t.Errorf("휴지통 대상 = %d건, want 3건 전부", len(tgt.TrashPaths))
	}
	if len(tgt.Excluded) != 0 {
		t.Errorf("--include-running인데 보존됨: %v", tgt.Excluded)
	}
	// 앱마다 한 번씩만 경고(경로마다 반복하면 소음이 된다).
	if len(tgt.Warnings) != 1 || !strings.Contains(tgt.Warnings[0], "Notion") {
		t.Errorf("경고 = %v, want Notion 1건", tgt.Warnings)
	}
}

// "3개 앱이 실행 중이라 제외됨 (총 12GB)"를 내려면 앱 단위 합계가 필요하다.
func TestRunningBlockedUsage(t *testing.T) {
	got := RunningBlockedUsage([]Entry{runningEntry()})
	if len(got) != 1 || got[0].Name != "Notion" {
		t.Fatalf("집계 = %+v, want Notion 1건", got)
	}
	if got[0].SizeKB != 41*1024*1024 { // Cache 40GB + GPUCache 1GB
		t.Errorf("합계 = %d KB, want %d", got[0].SizeKB, 41*1024*1024)
	}
	if RunningBlockedUsage(nil) != nil {
		t.Error("빈 입력인데 결과가 있음")
	}
}

// ---- 커버리지: Partitions / ScriptCache -----------------------------------

// 세션을 분리하는 앱은 Partitions/<파티션>/ 아래에 같은 구조를 또 만든다.
// 캐시는 잡히고 사용자 데이터는 Partitions 아래에서도 잡히면 안 된다.
func TestElectronPartitionsGlobs(t *testing.T) {
	home := t.TempDir()
	appSup := filepath.Join(home, "Library", "Application Support")
	mk := func(rel string) string {
		p := filepath.Join(appSup, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	want := []string{
		mk("Notion/Partitions/workspace-1/Cache"),
		mk("Notion/Partitions/workspace-1/Code Cache"),
		mk("Notion/Partitions/workspace-2/GPUCache"),
		mk("Slack/Partitions/team-a/Service Worker/CacheStorage"),
		mk("Notion/Service Worker/ScriptCache"),
	}
	bad := []string{
		mk("Notion/Partitions/workspace-1/IndexedDB"),
		mk("Notion/Partitions/workspace-1/Local Storage"),
		mk("Notion/Partitions/workspace-1/Cookies"),
		mk("Slack/Partitions/team-a/Service Worker/Database"),
	}

	got := ResolvePaths(home, []Location{electronLocation(t)})["electron-cache"]
	set := map[string]bool{}
	for _, p := range got {
		set[p] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("Partitions 캐시가 안 잡힘: %s", w)
		}
	}
	for _, b := range bad {
		if set[b] {
			t.Errorf("Partitions 아래 사용자 데이터가 잡힘: %s", b)
		}
	}
}

// Breakdown은 파티션 단위로 쪼개지면 안 된다 — 앱 단위로 합쳐야 판단이 된다.
func TestBreakdownAggregatesPartitionsPerApp(t *testing.T) {
	home := "/h"
	base := home + "/Library/Application Support/"
	sizes := map[string]int{
		base + "Notion/Cache":                             10 * 1024 * 1024,
		base + "Notion/Partitions/workspace-1/Cache":      20 * 1024 * 1024,
		base + "Notion/Partitions/workspace-2/Code Cache": 11 * 1024 * 1024,
		base + "Slack/Partitions/team-a/Cache":            3 * 1024 * 1024,
	}
	got := GroupUsage(home, GroupByAppSupportDir, sizes)
	if len(got) != 2 {
		t.Fatalf("그룹 = %+v, want 2개(Notion/Slack) — 파티션별로 쪼개지면 안 된다", got)
	}
	if got[0].Name != "Notion" || got[0].SizeKB != 41*1024*1024 {
		t.Errorf("Notion 합산 = %+v, want 41GB(10+20+11)", got[0])
	}
	if got[1].Name != "Slack" || got[1].SizeKB != 3*1024*1024 {
		t.Errorf("Slack = %+v", got[1])
	}

	// appSupportDirName이 Partitions 경로에서도 앱 이름을 뽑아야 한다.
	if n := appSupportDirName(home, base+"Notion/Partitions/workspace-1/Cache"); n != "Notion" {
		t.Errorf("appSupportDirName(Partitions 경로) = %q, want Notion", n)
	}
}

// 안전장치가 조용히 사라지면 안 된다.
// ps 조회에 실패하면 보호가 동작하지 않는데, 사용자는 도는 줄 알고 --apply를 누른다.
func TestPlanWarnsWhenRunningCheckFailed(t *testing.T) {
	electron := runningEntry()
	electron.GroupBy = GroupByAppSupportDir
	electron.BlockedByRunning = nil // 조회 실패라 아무것도 못 막았다
	npm := Entry{
		ID: "npm-cache", Name: "npm 캐시", Tier: TierSafe,
		Paths: []string{"/h/.npm/_cacache"}, SizeKB: 100,
		Cleanup: [][]string{{"npm", "cache", "clean", "--force"}},
	}

	hasWarning := func(p *clean.Plan) bool {
		for _, w := range p.Warnings {
			if strings.Contains(w, "실행 중인 앱을 확인할 수 없습니다") {
				return true
			}
		}
		return false
	}

	// ① 조회 실패 + Electron 항목 포함 → 경고
	failed := &Result{Entries: []Entry{electron, npm}, RunningCheckFailed: true}
	if !hasWarning(BuildCleanupPlan(failed, nil, false, false)) {
		t.Error("조회 실패인데 경고가 없음 — 보호가 조용히 꺼졌다")
	}

	// ② 조회 실패 + Electron 항목 없음 → 경고 없음(무관한 소음 방지)
	noElectron := &Result{Entries: []Entry{npm}, RunningCheckFailed: true}
	if hasWarning(BuildCleanupPlan(noElectron, nil, false, false)) {
		t.Error("Electron 항목이 없는데 경고가 붙음")
	}

	// ③ Electron 항목이 있어도 선택에서 빠지면 경고 없음
	onlyNpm := BuildCleanupPlan(failed, map[string]bool{"npm-cache": true}, false, false)
	if hasWarning(onlyNpm) {
		t.Error("계획에 안 담긴 위치 때문에 경고가 붙음")
	}

	// ④ 조회 성공 → 경고 없음
	okRes := &Result{Entries: []Entry{electron, npm}, RunningCheckFailed: false}
	if hasWarning(BuildCleanupPlan(okRes, nil, false, false)) {
		t.Error("조회 성공인데 경고가 붙음")
	}

	// 경고는 계획 렌더 **맨 앞**에 나와야 승인 전에 보인다(CLI·UI 공용 PlanLines 경유).
	lines := clean.PlanLines(BuildCleanupPlan(failed, nil, false, false))
	if len(lines) == 0 || lines[0].Kind != "planwarning" {
		t.Errorf("경고가 계획 상단에 없음: %+v", lines[:1])
	}
	text := clean.RenderPlanText(BuildCleanupPlan(failed, nil, false, false))
	if !strings.Contains(text, "실행 중인 앱을 확인할 수 없습니다") {
		t.Errorf("렌더 결과에 경고가 없음:\n%s", text)
	}
}

// 조회 실패와 "실행 중인 앱이 없음"은 반드시 구분돼야 한다.
func TestRunningCheckFailureIsNotEmptyList(t *testing.T) {
	// 빈 문자열 출력 = 실패로 본다(macOS에서 프로세스가 0개일 수는 없다).
	if apps := ParseRunningApps(""); len(apps) != 0 {
		t.Errorf("빈 출력인데 앱이 나옴: %v", apps)
	}
	// blockedByRunning은 실행 목록이 비면 아무것도 막지 않는다 —
	// 그래서 "비었음"을 실패와 구분해 Result에 따로 기록해야 한다.
	if got := blockedByRunning("/h", GroupByAppSupportDir, []string{"/h/x"}, nil); got != nil {
		t.Errorf("빈 실행 목록인데 차단됨: %v", got)
	}
}

package disk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// electronLocation은 locations.json의 Electron 캐시 항목을 찾아 준다.
func electronLocation(t *testing.T) Location {
	t.Helper()
	for _, l := range Locations() {
		if l.ID == "electron-cache" {
			return l
		}
	}
	t.Fatal("electron-cache 위치가 없음")
	return Location{}
}

// Electron 앱은 이름만 다를 뿐 같은 캐시 구조를 쓰므로 앱을 하드코딩하지 않고 패턴 하나로 잡는다.
func TestElectronCacheLocationShape(t *testing.T) {
	l := electronLocation(t)
	if l.Tier != TierSafe {
		t.Errorf("등급 = %q, want safe(재생성됨)", l.Tier)
	}
	if !l.Trash {
		t.Error("네이티브 정리 명령이 없으므로 trash여야 함")
	}
	if len(l.Cleanup) != 0 {
		t.Errorf("Electron 캐시에 네이티브 명령은 없다: %v", l.Cleanup)
	}
	if l.GroupBy != GroupByAppSupportDir {
		t.Errorf("group_by = %q, want %q (앱별 내역이 보여야 판단이 된다)", l.GroupBy, GroupByAppSupportDir)
	}
	// 앱 이름은 와일드카드여야 한다(Notion/Slack/… 하드코딩 금지).
	for _, g := range l.Globs {
		if !strings.Contains(g, "Application Support/*") {
			t.Errorf("앱 이름이 와일드카드가 아님: %s", g)
		}
	}
	want := []string{"Service Worker/CacheStorage", "Code Cache", "GPUCache", "Cache"}
	for _, w := range want {
		found := false
		for _, g := range l.Globs {
			if strings.HasSuffix(g, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("glob 누락: %s", w)
		}
	}
}

// 로그인 세션·로컬 데이터가 든 디렉토리는 **어떤 경우에도** 대상이 되면 안 된다.
// 지우면 재로그인이 필요해서, 이건 "캐시 정리"가 아니라 데이터 삭제다.
func TestElectronLocationNeverTargetsUserData(t *testing.T) {
	l := electronLocation(t)
	forbidden := []string{"IndexedDB", "Local Storage", "Cookies", "Session Storage", "databases"}
	for _, g := range l.Globs {
		for _, f := range forbidden {
			if strings.Contains(g, f) {
				t.Errorf("사용자 데이터 디렉토리가 glob에 있음(%s): %s", f, g)
			}
		}
	}
	// 제외 이유가 note에 남아 있어야 나중에 누가 무심코 추가하지 않는다.
	if !strings.Contains(l.Note, "IndexedDB") || !strings.Contains(l.Note, "재로그인") {
		t.Errorf("제외 이유가 note에 없음: %q", l.Note)
	}
}

// 가짜 Electron 앱 구조로 실제 매칭을 고정한다: 캐시는 잡히고 사용자 데이터는 안 잡힌다.
func TestElectronGlobsCatchCacheNotUserData(t *testing.T) {
	home := t.TempDir()
	appSup := filepath.Join(home, "Library", "Application Support")
	mk := func(rel string) string {
		p := filepath.Join(appSup, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// 잡혀야 하는 것.
	wantPaths := []string{
		mk("Notion/Cache"),
		mk("Notion/Code Cache"),
		mk("Notion/GPUCache"),
		mk("Notion/Service Worker/CacheStorage"),
		mk("Slack/Cache"),
	}
	// 절대 잡히면 안 되는 것.
	badPaths := []string{
		mk("Notion/IndexedDB"),
		mk("Notion/Local Storage"),
		mk("Notion/Cookies"),
		mk("Notion/Service Worker/Database"), // CacheStorage 옆의 실제 데이터
		mk("com.apple.Safari/Cache"),         // 애플 기본 앱 — 와일드카드가 삼키면 안 된다
	}

	got := ResolvePaths(home, []Location{electronLocation(t)})["electron-cache"]
	gotSet := map[string]bool{}
	for _, p := range got {
		gotSet[p] = true
	}
	for _, w := range wantPaths {
		if !gotSet[w] {
			t.Errorf("캐시가 안 잡힘: %s", w)
		}
	}
	for _, b := range badPaths {
		if gotSet[b] {
			t.Errorf("잡히면 안 되는 경로가 잡힘: %s", b)
		}
	}
}

func TestHasAppleComponent(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/Users/c/Library/Application Support/com.apple.Safari/Cache", true},
		{"/Users/c/Library/Containers/com.apple.mail/Data", true},
		{"/Users/c/Library/Application Support/Notion/Cache", false},
		{"/Users/c/Library/Application Support/com.appleseed.thing/Cache", false}, // 접두사만 비슷
		{"/Users/c/.npm/_cacache", false},
	}
	for _, c := range cases {
		if got := HasAppleComponent(c.path); got != c.want {
			t.Errorf("HasAppleComponent(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// 앱별 내역: 어느 앱이 얼마인지 보여야 판단이 된다(한 항목에 66GB만 뜨면 소용없다).
func TestGroupUsageAndBreakdownLine(t *testing.T) {
	home := "/Users/c"
	base := home + "/Library/Application Support/"
	sizes := map[string]int{
		base + "Notion/Cache":      40 * 1024 * 1024,
		base + "Notion/Code Cache": 1 * 1024 * 1024,
		base + "Slack/Cache":       3 * 1024 * 1024,
		base + "Discord/GPUCache":  512 * 1024,
		"/Users/c/.npm/_cacache":   99, // Application Support 밖 → 그룹 없음
	}
	got := GroupUsage(home, GroupByAppSupportDir, sizes)
	if len(got) != 3 {
		t.Fatalf("그룹 = %+v, want 3개(Notion/Slack/Discord)", got)
	}
	// 용량 내림차순이어야 한다.
	if got[0].Name != "Notion" || got[0].SizeKB != 41*1024*1024 {
		t.Errorf("첫 그룹 = %+v, want Notion 41GB(Cache+Code Cache 합산)", got[0])
	}
	if got[1].Name != "Slack" || got[2].Name != "Discord" {
		t.Errorf("정렬 오류: %+v", got)
	}

	line := BreakdownLine(got, 5)
	if !strings.Contains(line, "Notion 41.0 GB") || !strings.Contains(line, "Slack 3.0 GB") {
		t.Errorf("내역 줄 = %q", line)
	}
	// 상한을 넘으면 나머지는 접는다.
	if l := BreakdownLine(got, 1); !strings.Contains(l, "외 2개") {
		t.Errorf("접기 실패: %q", l)
	}
	// group_by가 없으면 내역도 없다.
	if u := GroupUsage(home, "", sizes); u != nil {
		t.Errorf("group_by 없는데 내역이 나옴: %v", u)
	}
	if BreakdownLine(nil, 5) != "" {
		t.Error("빈 내역인데 줄이 나옴")
	}
}

// 앱 단위 경로와 겹쳐도 이중 계산이 없어야 한다.
// disk 안에서는 ResolvePaths가 paths.DedupContained로 부모/자식을 한 번만 남긴다(6-4 규칙 재사용).
func TestElectronPathsNeverNest(t *testing.T) {
	home := t.TempDir()
	appSup := filepath.Join(home, "Library", "Application Support")
	for _, rel := range []string{"Notion/Cache", "Notion/Code Cache", "Notion/GPUCache", "Notion/Service Worker/CacheStorage"} {
		if err := os.MkdirAll(filepath.Join(appSup, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 앱 데이터 폴더 전체를 노리는 가상의 위치를 앞에 둔다(더 넓은 쪽이 이겨야 한다).
	locs := []Location{
		{ID: "whole-app", Globs: []string{"~/Library/Application Support/Notion"}, Tier: TierSafe},
		electronLocation(t),
	}
	got := ResolvePaths(home, locs)

	var all []string
	for _, ps := range got {
		all = append(all, ps...)
	}
	for _, a := range all {
		for _, b := range all {
			if a != b && strings.HasPrefix(b, a+string(filepath.Separator)) {
				t.Errorf("중첩된 경로가 함께 남음: %s ⊃ %s", a, b)
			}
		}
	}
	if len(got["electron-cache"]) != 0 {
		t.Errorf("더 넓은 위치가 있는데 하위 캐시가 따로 남음: %v", got["electron-cache"])
	}
}

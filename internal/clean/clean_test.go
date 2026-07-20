package clean

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsProtected(t *testing.T) {
	protected := []string{
		"/System",
		"/System/Applications/Safari.app",
		"/usr/bin/git",
		"/usr",
		"/bin/ls",
		"/sbin/ping",
		"/Library/Apple",
		"/Library/Apple/System/foo",
	}
	for _, p := range protected {
		if !IsProtected(p) {
			t.Errorf("IsProtected(%q) = false, want true (블랙리스트)", p)
		}
	}
	allowed := []string{
		"/Applications/Godot.app",
		"/Users/cris/Library/Caches/org.godotengine.godot",
		"/Users/cris/Applications/Foo.app",
		"/Library/LaunchAgents/com.example.plist", // /Library/Apple 이 아님
		"/usROOT/not-usr",                          // 접두사만 비슷 → 보호 아님
	}
	for _, p := range allowed {
		if IsProtected(p) {
			t.Errorf("IsProtected(%q) = true, want false", p)
		}
	}
}

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

func TestIsAppleSystemApp(t *testing.T) {
	apple := []string{"com.apple.Safari", "com.apple.dt.Xcode", "com.apple.finder"}
	for _, b := range apple {
		if !IsAppleSystemApp(b) {
			t.Errorf("IsAppleSystemApp(%q) = false, want true", b)
		}
	}
	notApple := []string{"org.godotengine.godot", "com.google.Chrome", "com.applesauce.app", ""}
	for _, b := range notApple {
		if IsAppleSystemApp(b) {
			t.Errorf("IsAppleSystemApp(%q) = true, want false", b)
		}
	}
}

func TestBundleGlobs(t *testing.T) {
	home := "/Users/cris"
	specs := bundleGlobs(home, "org.godotengine.godot")
	if len(specs) == 0 {
		t.Fatal("bundleGlobs가 비어 있음")
	}
	for _, s := range specs {
		if s.confidence != ConfHigh {
			t.Errorf("bundle glob은 모두 high여야 함: %+v", s)
		}
		// 접두사 매칭이어야 헬퍼/파생 파일을 잡는다: 사용자 홈 또는 /Library 아래.
		if !strings.Contains(s.pattern, "org.godotengine.godot") {
			t.Errorf("패턴에 Bundle ID 없음: %s", s.pattern)
		}
	}
	// Bundle ID 없으면 패턴 없음.
	if len(bundleGlobs(home, "")) != 0 {
		t.Errorf("Bundle ID 없는데 패턴 생성됨")
	}
}

// gatherLeftovers가 접두사 glob으로 헬퍼/파생 파일까지 잡는지 실제 파일로 검증.
func TestGatherLeftoversGlob(t *testing.T) {
	home := t.TempDir()
	mustCreate := func(rel string) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bundleID := "com.example.app"
	exact := mustCreate("Library/Preferences/" + bundleID + ".plist")
	helper := mustCreate("Library/LaunchAgents/" + bundleID + ".helper.plist") // 파생 파일
	cache := mustCreate("Library/Caches/" + bundleID)
	nameDir := mustCreate("Library/Application Support/ExampleApp/data.txt")
	unrelated := mustCreate("Library/Preferences/com.other.thing.plist")

	got := gatherLeftovers(home, bundleID, "ExampleApp")
	found := map[string]string{}
	for _, pm := range got {
		found[pm.Path] = pm.Confidence
	}

	if found[exact] != ConfHigh {
		t.Errorf("정확 일치 plist 미검출")
	}
	if found[helper] != ConfHigh {
		t.Errorf("파생 파일(<bundleID>.helper.plist) 미검출 — 접두사 glob 실패")
	}
	if found[cache] != ConfHigh {
		t.Errorf("캐시 미검출")
	}
	// 이름 기반 디렉토리는 low 확신으로 잡혀야 함(부모 디렉토리).
	if found[filepath.Join(home, "Library/Application Support/ExampleApp")] != ConfLow {
		t.Errorf("이름 기반 디렉토리 미검출/확신도 오류: %v", found)
	}
	_ = nameDir
	if _, ok := found[unrelated]; ok {
		t.Errorf("무관한 파일이 잡힘: %s", unrelated)
	}
}

package paths

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
		"/usROOT/not-usr",                         // 접두사만 비슷 → 보호 아님
	}
	for _, p := range allowed {
		if IsProtected(p) {
			t.Errorf("IsProtected(%q) = true, want false", p)
		}
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

// derivedBundleID: 잔여물 파일명에서 Bundle ID 후보를 정확히 뽑아야 접두사 과매칭을 걷어낼 수 있다.
func TestDerivedBundleID(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/Users/c/Library/Preferences/com.google.Chrome.plist", "com.google.Chrome"},
		{"/Users/c/Library/Caches/com.google.Chrome.canary", "com.google.Chrome.canary"},
		{"/Users/c/Library/LaunchAgents/com.google.Chrome.helper.plist", "com.google.Chrome.helper"},
		{"/Users/c/Library/Saved Application State/com.google.Chrome.savedState", "com.google.Chrome"},
		{"/Users/c/Library/Cookies/com.google.Chrome.binarycookies", "com.google.Chrome"},
		{"/Users/c/Library/Group Containers/EQHXZ8M8AV.com.google.Chrome", "com.google.Chrome"},
		{"/Users/c/Library/Group Containers/group.com.example.app", "com.example.app"},
		// ByHost의 호스트 UUID 접미사는 ID의 일부가 아니다.
		{"/Users/c/Library/Preferences/ByHost/com.example.app.2CD92239-1111-2222-3333-444455556666.plist", "com.example.app"},
	}
	for _, c := range cases {
		if got := DerivedBundleID(c.path); got != c.want {
			t.Errorf("DerivedBundleID(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// 앱 이름에 glob 메타문자가 있어도 무관한 경로를 삼키면 안 된다(이름 경로는 리터럴 매칭).
func TestNameGlobsMetacharacters(t *testing.T) {
	home := t.TempDir()
	caches := filepath.Join(home, "Library", "Caches")
	if err := os.MkdirAll(caches, 0o755); err != nil {
		t.Fatal(err)
	}
	// 문자 클래스/와일드카드로 해석되면 잡혔을 미끼 경로들.
	for _, decoy := range []string{"Foo b", "Foo e", "PhotosLibrary", "Whatx"} {
		if err := os.Mkdir(filepath.Join(caches, decoy), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 진짜 이름의 디렉토리(리터럴)는 잡혀야 한다.
	real := filepath.Join(caches, "Foo [beta]")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Foo [beta]", "Photos*", "What?"} {
		for _, pm := range gatherLeftovers(home, "", name) {
			if pm.Path != real {
				t.Errorf("앱 이름 %q이 무관한 경로를 매칭함: %s", name, pm.Path)
			}
		}
	}
	// "Foo [beta]"는 리터럴 경로로 자기 자신을 찾아야 한다.
	found := false
	for _, pm := range gatherLeftovers(home, "", "Foo [beta]") {
		if pm.Path == real {
			found = true
		}
	}
	if !found {
		t.Error("리터럴 이름 경로를 못 찾음: Foo [beta]")
	}
}

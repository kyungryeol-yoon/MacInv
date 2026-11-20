package disk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

func TestLocationsEmbedLoads(t *testing.T) {
	locs := Locations()
	if len(locs) == 0 {
		t.Fatal("임베드된 locations.json이 비었음")
	}
	seen := map[string]bool{}
	validTier := map[string]bool{TierSafe: true, TierArtifact: true, TierExpensive: true, TierNever: true}
	for _, l := range locs {
		if l.ID == "" || l.Name == "" || len(l.Globs) == 0 {
			t.Errorf("필수 필드 누락: %+v", l)
		}
		if seen[l.ID] {
			t.Errorf("ID 중복: %s", l.ID)
		}
		seen[l.ID] = true
		if !validTier[l.Tier] {
			t.Errorf("%s: 알 수 없는 등급 %q", l.ID, l.Tier)
		}
		// 정리 수단이 없으면(리포트 전용) 왜 그런지 note로 설명해야 사용자가 막다른 길에 안 걸린다.
		if len(l.Cleanup) == 0 && !l.Trash && l.Note == "" {
			t.Errorf("%s: 정리 명령도 trash도 note도 없음", l.ID)
		}
		// 정리 명령은 argv 배열이어야 한다(문자열 한 덩어리 금지 — 6-8 규칙).
		for _, argv := range l.Cleanup {
			if len(argv) == 0 {
				t.Errorf("%s: 빈 명령", l.ID)
			}
		}
	}
	// 스펙에 명시된 초기 항목이 빠지지 않았는지.
	for _, want := range []string{
		"ollama-models", "huggingface-cache", "lmstudio", "orbstack-data", "docker-data",
		"npm-cache", "pnpm-store", "cargo-registry", "go-modcache", "gradle-cache",
		"maven-repo", "pip-cache", "homebrew-cache",
		"xcode-deriveddata", "xcode-devicesupport", "coresimulator",
	} {
		if !seen[want] {
			t.Errorf("초기 항목 누락: %s", want)
		}
	}
}

// 기본 정리 대상: safe+artifact. expensive는 옵트인, never는 어떤 경우에도 제외.
func TestCleanable(t *testing.T) {
	cases := []struct {
		tier             string
		includeExpensive bool
		want             bool
	}{
		{TierSafe, false, true},
		{TierArtifact, false, true},
		{TierExpensive, false, false},
		{TierExpensive, true, true},
		{TierNever, false, false},
		{TierNever, true, false}, // never는 플래그로도 뚫리지 않는다
		{"unknown", true, false},
	}
	for _, c := range cases {
		if got := Cleanable(c.tier, c.includeExpensive); got != c.want {
			t.Errorf("Cleanable(%q, %v) = %v, want %v", c.tier, c.includeExpensive, got, c.want)
		}
	}
}

// 고아 캐시 = 캐시는 있는데 도구가 없다. 보수적으로 판정한다.
func TestIsOrphanCache(t *testing.T) {
	installed := map[string]bool{"go": true, "brew": true}
	cases := []struct {
		name string
		loc  Location
		want bool
	}{
		{"도구 없음 → 고아", Location{Tool: "gradle", Tier: TierSafe}, true},
		{"도구 있음 → 고아 아님", Location{Tool: "go", Tier: TierSafe}, false},
		{"대소문자 무시", Location{Tool: "Brew", Tier: TierSafe}, false},
		{"소유 도구를 모르면 판정 안 함", Location{Tool: "", Tier: TierSafe}, false},
		{"never 등급은 고아 표시 안 함", Location{Tool: "gradle", Tier: TierNever}, false},
		{"expensive도 고아 판정 대상", Location{Tool: "lms", Tier: TierExpensive}, true},
	}
	for _, c := range cases {
		if got := IsOrphanCache(c.loc, installed); got != c.want {
			t.Errorf("%s: IsOrphanCache = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestInstalledTools(t *testing.T) {
	rep := &report.Report{
		Sections: report.Sections{
			Brew: &report.Brew{Formulae: []report.Formula{{Name: "git"}}},
			Apps: []report.App{{Name: "OrbStack", BundleID: report.Ptr("dev.kdrag0n.MacVirt")}},
		},
		Runtimes: []report.Runtime{{Name: "python", Installed: true}, {Name: "ruby", Installed: false}},
	}
	got := InstalledTools(rep)

	for _, want := range []string{
		"git",
		"brew",     // 섹션 존재 자체가 근거 — 포뮬러 목록에는 brew가 없다
		"orbstack", // 앱 이름
		"macvirt",  // Bundle ID 마지막 마디
		"python",
		"pip", // 별칭: python이 있으면 pip도 있다
	} {
		if !got[want] {
			t.Errorf("설치 도구에 %q가 없음: %v", want, got)
		}
	}
	if got["ruby"] {
		t.Error("미설치 런타임이 설치로 잡힘")
	}
	// nil 리포트에도 죽지 않는다(캐시가 없을 때).
	if len(InstalledTools(nil)) != 0 {
		t.Error("nil 리포트인데 도구가 나옴")
	}
}

func TestExpandGlob(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ExpandGlob(home, "~/.cache/foo")
	if len(got) != 1 || got[0] != filepath.Join(home, ".cache/foo") {
		t.Errorf("~ 확장 실패: %v", got)
	}
	if g := ExpandGlob(home, "~/.cache/*"); len(g) != 1 {
		t.Errorf("glob 확장 실패: %v", g)
	}
	// 존재하지 않는 경로는 결과에 없다(미설치는 에러가 아니라 조용한 스킵).
	if g := ExpandGlob(home, "~/.nope"); len(g) != 0 {
		t.Errorf("없는 경로가 잡힘: %v", g)
	}
}

// 부모/자식이 겹치면 du가 두 번 세므로 한 번만 남아야 한다(6-4 규칙 재사용).
func TestResolvePathsDedup(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".tool", "caches"), 0o755); err != nil {
		t.Fatal(err)
	}
	locs := []Location{
		{ID: "parent", Globs: []string{"~/.tool"}, Tier: TierSafe},
		{ID: "child", Globs: []string{"~/.tool/caches"}, Tier: TierSafe},
	}
	got := ResolvePaths(home, locs)
	if len(got["parent"]) != 1 {
		t.Errorf("부모 위치가 비었음: %v", got)
	}
	if len(got["child"]) != 0 {
		t.Errorf("자식 경로가 따로 남아 이중 계산됨: %v", got["child"])
	}
}

func TestParseOllamaList(t *testing.T) {
	out := "NAME                 ID              SIZE      MODIFIED     \n" +
		"llama3:8b            365c0bd3c000    4.7 GB    3 days ago      \n" +
		"qwen2.5-coder:14b    9ec8897f747e    9.0 GB    2 weeks ago     \n" +
		"tiny:latest          aaaabbbbcccc    512 MB    1 hour ago      \n"
	got := ParseOllamaList(out)
	if len(got) != 3 {
		t.Fatalf("모델 수 = %d, want 3 (%v)", len(got), got)
	}
	// 4.7 GB → KB (소수 곱이라 절삭된 값)
	const want47GB = 4928307
	if got[0].Name != "llama3:8b" || got[0].SizeKB != want47GB {
		t.Errorf("첫 모델 파싱 오류: %+v", got[0])
	}
	if got[2].SizeKB != 512*1024 {
		t.Errorf("MB 단위 파싱 오류: %+v", got[2])
	}
	// 헤더 줄은 모델이 아니다.
	for _, m := range got {
		if m.Name == "NAME" {
			t.Error("헤더가 모델로 파싱됨")
		}
	}
}

func TestParseSnapshotsAndDF(t *testing.T) {
	out := "Snapshots for disk /:\n" +
		"com.apple.TimeMachine.2026-08-01-120000.local\n" +
		"com.apple.TimeMachine.2026-08-02-090000.local\n"
	names := ParseSnapshots(out)
	if len(names) != 2 {
		t.Errorf("스냅샷 = %v, want 2개", names)
	}
	// 스냅샷이 없으면 헤더만 나온다.
	if n := ParseSnapshots("Snapshots for disk /:\n"); len(n) != 0 {
		t.Errorf("빈 목록인데 %v", n)
	}

	df := "Filesystem   1024-blocks      Used Available Capacity  Mounted on\n" +
		"/dev/disk3s5   239362496 133169152  70123456    66%    /System/Volumes/Data\n"
	used, avail := ParseDF(df)
	if used != 133169152 || avail != 70123456 {
		t.Errorf("ParseDF = (%d, %d)", used, avail)
	}
	if u, a := ParseDF("garbage"); u != 0 || a != 0 {
		t.Errorf("이상한 입력인데 값이 나옴: %d %d", u, a)
	}
}

func TestSumDuKB(t *testing.T) {
	out := "1024\t/a\n2048\t/b/with space\nbroken\n"
	if got := SumDuKB(out); got != 3072 {
		t.Errorf("SumDuKB = %d, want 3072", got)
	}
}

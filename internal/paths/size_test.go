package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupContainerMatchesApp(t *testing.T) {
	tests := []struct {
		folder  string
		appName string
		want    bool
	}{
		// 실기 사례: OrbStack의 Bundle ID는 dev.kdrag0n.MacVirt라 ID 매칭으로는 못 찾는다.
		{"HUAQ24HBR6.dev.orbstack", "OrbStack", true},
		{"group.com.apple.podcasts", "Podcasts", true},
		{"243LU875E5.groups.com.apple.podcasts", "Podcasts", true},
		{"com.company.SomeApp", "Some App", true}, // 공백 무시
		{"com.company.some-app", "SomeApp", true}, // 하이픈 무시
		// 마지막 마디만 정확 비교 — 부분일치로 무관한 컨테이너를 쓸어담지 않는다.
		{"group.com.apple.mail.notes", "Mail", false},
		{"group.com.apple.calendar", "Mail", false},
		{"com.apple.Home.group", "Home", false}, // 마지막 마디는 "group"
		{"HUAQ24HBR6.dev.orbstack", "Orb", false},
		// 마디가 없는 이름은 Bundle ID 꼴이 아니므로 대상 아님.
		{"Notion", "Notion", false},
		{"group.com.apple.calendar", "", false},
	}
	for _, tt := range tests {
		if got := groupContainerMatchesApp(tt.folder, tt.appName); got != tt.want {
			t.Errorf("groupContainerMatchesApp(%q, %q) = %v, want %v", tt.folder, tt.appName, got, tt.want)
		}
	}
}

func TestVendorAppDirs(t *testing.T) {
	// Chrome 데이터는 벤더 폴더 아래(~/Library/Application Support/Google/Chrome)에 있다.
	paths := vendorAppDirs("/home/u", "com.google.Chrome", "Google Chrome")
	want := "/home/u/Library/Application Support/Google/Chrome"
	found := false
	for _, p := range paths {
		if p == want {
			found = true
		}
		// 벤더 폴더 전체(공유 컴포넌트 포함)를 이 앱 용량으로 잡으면 안 된다.
		if p == "/home/u/Library/Application Support/Google" {
			t.Errorf("벤더 폴더 전체가 포함됨: %s", p)
		}
	}
	if !found {
		t.Errorf("%s 없음: %v", want, paths)
	}
	if got := vendorAppDirs("/home/u", "com.example.unknown", "Unknown"); got != nil {
		t.Errorf("등록되지 않은 벤더는 nil이어야 함: %v", got)
	}
}

// du -sk에 부모와 자식이 함께 넘어가면 자식이 두 번 계산된다 — 양방향으로 걸러야 한다.
func TestDedupContained(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		// 부모가 나중에 들어오는 경우(예전 버그: 자식이 남아 이중 계산).
		{"부모가 나중", []string{"/A/B", "/A"}, []string{"/A"}},
		// 자식이 나중에 들어오는 경우.
		{"자식이 나중", []string{"/A", "/A/B"}, []string{"/A"}},
		// 부모 하나가 여러 자식을 걷어낸다.
		{"자식 여러 개", []string{"/A/B", "/A/C/D", "/A"}, []string{"/A"}},
		// 완전 중복.
		{"같은 경로", []string{"/A", "/A"}, []string{"/A"}},
		// 형제는 둘 다 남는다.
		{"형제", []string{"/A/B", "/A/C"}, []string{"/A/B", "/A/C"}},
		// 이름이 접두사일 뿐 부모/자식이 아니면 남는다(/A와 /AB는 무관).
		{"접두사만 유사", []string{"/A", "/AB"}, []string{"/A", "/AB"}},
		{"빈 입력", nil, nil},
	}
	for _, c := range cases {
		in := make([]PathMatch, 0, len(c.in))
		for _, p := range c.in {
			in = append(in, PathMatch{Path: p, Confidence: ConfHigh})
		}
		got := DedupContained(in)
		if len(got) != len(c.want) {
			t.Errorf("%s: DedupContained(%v) = %v, want %v", c.name, c.in, pathsOf(got), c.want)
			continue
		}
		for i, w := range c.want {
			if got[i].Path != w {
				t.Errorf("%s: [%d] = %s, want %s", c.name, i, got[i].Path, w)
			}
		}
	}
}

func pathsOf(ms []PathMatch) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Path)
	}
	return out
}

// SizePaths 결과에는 부모/자식이 함께 들어가면 안 된다(실기 회귀 방어).
func TestSizePathsNoNesting(t *testing.T) {
	home := t.TempDir()
	// 벤더 폴더 전체(~/Library/Application Support/Google)와 그 하위(Chrome)가 모두 후보가 되는 상황.
	for _, rel := range []string{
		"Library/Application Support/com.google.Chrome",
		"Library/Application Support/com.google.Chrome/Cache",
		"Library/Application Support/Google/Chrome",
	} {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := SizePaths(home, "com.google.Chrome", "Google Chrome")
	for i, a := range got {
		for j, b := range got {
			if i != j && isUnder(b.Path, a.Path) {
				t.Errorf("중첩된 경로가 함께 측정 대상에 들어감: %s ⊃ %s", a.Path, b.Path)
			}
		}
	}
}

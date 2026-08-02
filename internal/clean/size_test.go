package clean

import "testing"

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

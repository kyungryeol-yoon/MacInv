package disk

import (
	"testing"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// 핵심 회귀: CLI 도구는 없어도 GUI 앱이 그 캐시를 쓰면 고아가 아니다.
// gradle CLI 없이 Android Studio만 깔아도 ~/.gradle은 살아있는 캐시다.
// (이 맥에 해당 앱이 없어 실기로는 확인되지 않으므로 테이블로 고정한다.)
func TestIsOrphanCacheOwnerApps(t *testing.T) {
	gradle := Location{ID: "gradle-cache", Tool: "gradle", Tier: TierSafe,
		OwnerApps: []string{"Android Studio", "IntelliJ IDEA"}}
	maven := Location{ID: "maven-repo", Tool: "mvn", Tier: TierSafe,
		OwnerApps: []string{"IntelliJ IDEA"}}

	cases := []struct {
		name      string
		loc       Location
		installed map[string]bool
		want      bool
	}{
		{
			name:      "gradle 미설치 + Android Studio 설치 → 고아 아님",
			loc:       gradle,
			installed: map[string]bool{"android studio": true, "androidstudio": true},
			want:      false,
		},
		{
			name:      "gradle 미설치 + 공백 없는 키만 있어도 인식",
			loc:       gradle,
			installed: map[string]bool{"androidstudio": true},
			want:      false,
		},
		{
			name:      "gradle 미설치 + 소유 앱도 없음 → 고아",
			loc:       gradle,
			installed: map[string]bool{"go": true, "brew": true},
			want:      true,
		},
		{
			name:      "gradle CLI만 있어도 고아 아님",
			loc:       gradle,
			installed: map[string]bool{"gradle": true},
			want:      false,
		},
		{
			name:      "소유 앱 중 하나만 있어도 고아 아님",
			loc:       maven,
			installed: map[string]bool{"intellij idea": true},
			want:      false,
		},
		{
			name:      "무관한 앱은 구제하지 않는다",
			loc:       maven,
			installed: map[string]bool{"visual studio code": true},
			want:      true,
		},
		{
			name:      "owner_apps 없는 위치는 도구만 본다",
			loc:       Location{Tool: "npm", Tier: TierSafe},
			installed: map[string]bool{},
			want:      true,
		},
	}
	for _, c := range cases {
		if got := IsOrphanCache(c.loc, c.installed); got != c.want {
			t.Errorf("%s: IsOrphanCache = %v, want %v", c.name, got, c.want)
		}
	}
}

// 넣을 때(InstalledTools)와 찾을 때(IsOrphanCache)가 같은 정규화를 써야 한다.
// 한쪽만 바뀌면 매칭이 조용히 깨져 오탐이 돌아온다.
func TestInstalledToolsAndLookupUseSameKeys(t *testing.T) {
	rep := &report.Report{Sections: report.Sections{
		Apps: []report.App{{Name: "Android Studio"}, {Name: "IntelliJ IDEA"}},
	}}
	installed := InstalledTools(rep)

	gradle := Location{Tool: "gradle", Tier: TierSafe, OwnerApps: []string{"Android Studio"}}
	if IsOrphanCache(gradle, installed) {
		t.Errorf("Android Studio가 설치됐는데 gradle 캐시가 고아로 잡힘: %v", installed)
	}
	maven := Location{Tool: "mvn", Tier: TierSafe, OwnerApps: []string{"IntelliJ IDEA"}}
	if IsOrphanCache(maven, installed) {
		t.Error("IntelliJ IDEA가 설치됐는데 m2가 고아로 잡힘")
	}
}

func TestToolKeys(t *testing.T) {
	cases := map[string][]string{
		"Android Studio": {"android studio", "androidstudio"},
		"gradle":         {"gradle"},
		"IntelliJ IDEA":  {"intellij idea", "intellijidea"},
		"  Xcode  ":      {"xcode"},
		"":               nil,
	}
	for in, want := range cases {
		got := toolKeys(in)
		if len(got) != len(want) {
			t.Errorf("toolKeys(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("toolKeys(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

// locations.json의 owner_apps가 실제로 로드되는지(스키마 연결 확인).
func TestOwnerAppsLoadedFromJSON(t *testing.T) {
	want := map[string]string{"gradle-cache": "Android Studio", "maven-repo": "IntelliJ IDEA"}
	for id, app := range want {
		found := false
		for _, l := range Locations() {
			if l.ID != id {
				continue
			}
			for _, a := range l.OwnerApps {
				if a == app {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s의 owner_apps에 %q가 없음", id, app)
		}
	}
}

// PATH 2차 확인: 인벤토리에 없어도 PATH에 실행 파일이 있으면 설치로 본다.
// 인벤토리만 믿으면 캐시가 오래됐을 때 멀쩡한 도구를 고아로 오탐한다.
func TestDetectToolsChecksPATH(t *testing.T) {
	locs := []Location{
		{ID: "shell-thing", Tool: "sh", Tier: TierSafe},                        // PATH에 반드시 있다
		{ID: "nope", Tool: "definitely-not-a-real-binary-xyz", Tier: TierSafe}, // 없다
	}
	got := detectToolsFrom(nil, locs) // 인벤토리 없음 → PATH만으로 판단

	if !got["sh"] {
		t.Error("PATH에 있는 sh가 설치로 안 잡힘")
	}
	if got["definitely-not-a-real-binary-xyz"] {
		t.Error("없는 바이너리가 설치로 잡힘")
	}
	if IsOrphanCache(locs[0], got) {
		t.Error("PATH에 있는 도구의 캐시가 고아로 잡힘")
	}
	if !IsOrphanCache(locs[1], got) {
		t.Error("정말 없는 도구인데 고아로 안 잡힘")
	}
}

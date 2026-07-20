package clean

import (
	"strings"
	"testing"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

func TestClassifyOrphan(t *testing.T) {
	installed := []string{"com.google.Chrome", "com.googlecode.iterm2", "org.godotengine.godot"}
	vs := vendors()

	cases := []struct {
		name string
		id   string
		want bool // true = 고아
	}{
		{"설치 앱 정확 일치", "com.googlecode.iterm2", false},
		{"설치 앱 파생물(하위)", "com.googlecode.iterm2.helper", false},
		{"설치 ID가 항목의 접두사(역방향)", "com.googlecode", false},
		{"애플은 무조건 보존", "com.apple.dock", false},
		{"팀ID 접두사 애플 그룹 보존", "243LU875E5.groups.com.apple.podcasts", false},
		{"systemgroup 보존", "systemgroup.com.apple.icloud.searchpartyd.sharedsettings", false},
		{"Apple 단축어(is.workflow) 보존", "is.workflow.my.app", false},
		{"Sparkle 공유 프레임워크 보존", "org.sparkle-project.Sparkle.Autoupdate", false},
		{"팀ID 접두사는 비애플이라도 보존", "ABCDE12345.com.some.group", false},
		{"벤더 공유 — Chrome 설치 중이면 keystone 보존", "com.google.keystone.agent", false},
		{"진짜 고아", "com.spotify.client", true},
		{"진짜 고아 2", "org.deleted.oldapp", true},
		{"이름 폴더(비 reverse-DNS)는 제외", "Notion", false},
		{"점 1개는 형태 미달로 제외", "com.spotify", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyOrphan(c.id, installed, vs); got != c.want {
				t.Errorf("classifyOrphan(%q) = %v, want %v", c.id, got, c.want)
			}
		})
	}

	// 구글 앱이 하나도 없으면 keystone은 고아가 된다.
	noGoogle := []string{"com.googlecode.iterm2"}
	if !classifyOrphan("com.google.keystone.agent", noGoogle, vs) {
		t.Errorf("구글 앱 전무한데 keystone이 고아가 아님")
	}
}

func TestExtractOrphanID(t *testing.T) {
	cases := []struct {
		name, strip, want string
	}{
		{"com.spotify.client.plist", ".plist", "com.spotify.client"},
		{"notaplist.txt", ".plist", ""}, // 접미사 요구 미충족 → 대상 아님
		{"com.spotify.client", "", "com.spotify.client"},
		{"com.old.app.savedState", ".savedState", "com.old.app"},
		// ByHost UUID 접미사 제거
		{"notion.id.ShipIt.2CD92239-CF12-5BD9-AF3A-C07DA9877FD9.plist", ".plist", "notion.id.ShipIt"},
		// Group Containers의 group. 접두사 정규화
		{"group.com.old.app", "", "com.old.app"},
	}
	for _, c := range cases {
		if got := extractOrphanID(c.name, c.strip); got != c.want {
			t.Errorf("extractOrphanID(%q,%q) = %q, want %q", c.name, c.strip, got, c.want)
		}
	}
}

func TestUpgradePlanFromOutdated(t *testing.T) {
	rows := []report.OutdatedRow{
		{Source: "brew", Name: "git", Current: "2.53", Latest: "2.55"},
		{Source: "cask", Name: "iterm2", Current: "3.5.6", Latest: "3.5.9"},
		{Source: "npm", Name: "npm", Current: "10", Latest: "12"},
		{Source: "mas", Name: "Xcode", Current: "26.4.0", Latest: "26.4.1", ID: "497799835"},
		{Source: "mas", Name: "NoID", Current: "1", Latest: "2"}, // ID 없음 → 제외
	}
	plan := UpgradePlanFromOutdated(rows)
	if plan.Verb != "업데이트" || len(plan.Targets) != 4 {
		t.Fatalf("타깃 수 = %d, want 4 (ID 없는 mas 제외)", len(plan.Targets))
	}
	joined := ""
	for _, tgt := range plan.Targets {
		if tgt.CmdTimeoutSec != upgradeTimeoutSec {
			t.Errorf("%s 타임아웃 = %d, want %d", tgt.Name, tgt.CmdTimeoutSec, upgradeTimeoutSec)
		}
		joined += strings.Join(tgt.Commands, ";") + ";"
	}
	for _, want := range []string{
		"brew upgrade git", "brew upgrade --cask iterm2",
		"npx -y npm@latest install -g npm@latest", // npm 셀프 → npx 규칙 자동 적용
		"mas upgrade 497799835",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("명령 누락: %q\n%s", want, joined)
		}
	}
	// 빈 입력 → 빈 계획.
	if UpgradePlanFromOutdated(nil).HasContent() {
		t.Errorf("빈 rows인데 계획에 내용 있음")
	}
}

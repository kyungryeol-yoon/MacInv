package paths

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

// teamIDPrefixRe는 개발자 팀 ID 접두사다(예: HUAQ24HBR6.dev.orbstack, UBF8T346G9.Office).
var teamIDPrefixRe = regexp.MustCompile(`^[A-Z0-9]{10}\.`)

// uuidSuffixRe는 ByHost plist의 호스트 UUID 접미사다(com.x.y.2CD92239-....plist).
var uuidSuffixRe = regexp.MustCompile(`\.[0-9A-Fa-f]{8}-[0-9A-Fa-f-]{27}$`)

// HasTeamIDPrefix는 이름이 팀 ID 접두사로 시작하는지 본다(순수 함수).
func HasTeamIDPrefix(name string) bool { return teamIDPrefixRe.MatchString(name) }

// StripTeamIDPrefix는 팀 ID 접두사를 벗긴다(순수 함수).
func StripTeamIDPrefix(name string) string { return teamIDPrefixRe.ReplaceAllString(name, "") }

// StripHostUUID는 ByHost plist 이름의 호스트 UUID 접미사를 벗긴다(순수 함수).
func StripHostUUID(name string) string { return uuidSuffixRe.ReplaceAllString(name, "") }

// leftoverSuffixes는 잔여물 파일명 끝에 붙는 확장자다(파생 ID 추출 시 벗긴다).
var leftoverSuffixes = []string{".plist", ".savedState", ".binarycookies"}

// DerivedBundleID는 잔여물 경로 이름에서 Bundle ID 후보를 뽑는다(순수 함수 — 테스트 대상).
// 확장자·ByHost UUID 접미사·그룹 컨테이너의 팀ID/group. 접두사를 정규화해 벗긴다.
// clean이 접두사 과매칭(다른 앱 소유 경로)을 걷어낼 때 쓴다.
func DerivedBundleID(path string) string {
	name := filepath.Base(path)
	for _, s := range leftoverSuffixes {
		if strings.HasSuffix(name, s) {
			name = strings.TrimSuffix(name, s)
			break
		}
	}
	name = StripHostUUID(name)
	name = StripTeamIDPrefix(name)
	return strings.TrimPrefix(name, "group.")
}

// BundleIDFromPlist는 앱 번들의 Info.plist에서 CFBundleIdentifier를 직접 읽는다.
// (바이너리 plist도 있어 plutil을 쓴다.) mdls가 (null)을 주는 앱의 폴백이다.
func BundleIDFromPlist(ctx context.Context, run *iexec.Runner, appPath string) string {
	plist := filepath.Join(appPath, "Contents", "Info.plist")
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 10},
		"plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", plist)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

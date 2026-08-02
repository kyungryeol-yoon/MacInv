package paths

import (
	"path/filepath"
)

// LeftoverPaths는 앱이 ~/Library 등지에 남긴 데이터 경로를 돌려준다(읽기 전용).
// 삭제 계획과 같은 매칭 규칙을 쓰므로, scan의 용량 집계가 "clean이 지울 대상"과 일치한다.
func LeftoverPaths(home, bundleID, appName string) []PathMatch {
	return gatherLeftovers(home, bundleID, appName)
}

// globSpec은 잔여물 후보를 찾는 패턴 하나다(존재 확인 전).
type globSpec struct {
	pattern    string
	confidence string
	reason     string
}

// bundleGlobs는 Bundle ID로 잔여물 glob 패턴을 만든다(순수 함수 — 테스트 대상).
// 접두사 매칭(<B>*)이라 헬퍼/파생 파일(com.vendor.app.helper.plist 등)까지 잡는다. 모두 높은 확신.
// 접두사가 다른 앱의 Bundle ID까지 삼키는 문제는 clean.excludeOtherAppIDs가 사후에 걷어낸다.
func bundleGlobs(home, rawBundleID string) []globSpec {
	if rawBundleID == "" {
		return nil
	}
	bundleID := escapeGlobMeta(rawBundleID)
	lib := filepath.Join(home, "Library")
	p := func(base, sub, suffix string) string { return filepath.Join(base, sub, bundleID+suffix) }
	return []globSpec{
		{p(lib, "Preferences", "*.plist"), ConfHigh, "환경설정 (Bundle ID)"},
		{p(lib, "Preferences/ByHost", "*.plist"), ConfHigh, "환경설정 ByHost (Bundle ID)"},
		{p(lib, "Caches", "*"), ConfHigh, "캐시 (Bundle ID)"},
		{p(lib, "HTTPStorages", "*"), ConfHigh, "HTTP 저장소 (Bundle ID)"},
		{p(lib, "WebKit", "*"), ConfHigh, "WebKit 데이터 (Bundle ID)"},
		{p(lib, "Containers", "*"), ConfHigh, "샌드박스 컨테이너 (Bundle ID)"},
		{p(lib, "Application Scripts", "*"), ConfHigh, "앱 스크립트 (Bundle ID)"},
		{p(lib, "Saved Application State", "*.savedState"), ConfHigh, "저장된 상태 (Bundle ID)"},
		{p(lib, "Application Support", "*"), ConfHigh, "앱 지원 데이터 (Bundle ID)"},
		{p(lib, "Logs", "*"), ConfHigh, "로그 (Bundle ID)"},
		{p(lib, "Cookies", "*.binarycookies"), ConfHigh, "쿠키 (Bundle ID)"},
		{p(lib, "LaunchAgents", "*.plist"), ConfHigh, "LaunchAgent (Bundle ID)"},
		{filepath.Join(lib, "Group Containers", "*"+bundleID+"*"), ConfHigh, "그룹 컨테이너 (Bundle ID)"},
		// 시스템 전역(/Library) — 삭제에 관리자 권한이 필요할 수 있음.
		{filepath.Join("/Library", "Application Support", bundleID+"*"), ConfHigh, "앱 지원 (시스템 · 권한 필요할 수 있음)"},
		{filepath.Join("/Library", "Caches", bundleID+"*"), ConfHigh, "캐시 (시스템)"},
		{filepath.Join("/Library", "Preferences", bundleID+"*.plist"), ConfHigh, "환경설정 (시스템)"},
		{filepath.Join("/Library", "LaunchAgents", bundleID+"*.plist"), ConfHigh, "LaunchAgent (시스템)"},
		{filepath.Join("/Library", "LaunchDaemons", bundleID+"*.plist"), ConfHigh, "LaunchDaemon (시스템)"},
	}
}

// nameGlobs는 앱 이름 기반 잔여물 경로를 만든다(순수 함수 — 테스트 대상). 낮은 확신.
// 여기 pattern은 glob이 아니라 **리터럴 경로**다(gatherLeftovers가 존재 확인만 한다) —
// "Foo [beta]"/"Photos*"/"What?" 같은 이름이 문자 클래스·와일드카드로 해석돼
// 무관한 경로(~/Library/Caches/Foo b 등)를 삼키면 안 되기 때문이다.
func nameGlobs(home, appName string) []globSpec {
	if appName == "" {
		return nil
	}
	lib := filepath.Join(home, "Library")
	return []globSpec{
		{filepath.Join(lib, "Application Support", appName), ConfLow, "앱 지원 데이터 (이름 기반 · 확신 낮음)"},
		{filepath.Join(lib, "Caches", appName), ConfLow, "캐시 (이름 기반 · 확신 낮음)"},
		{filepath.Join(lib, "Logs", appName), ConfLow, "로그 (이름 기반 · 확신 낮음)"},
	}
}

// gatherLeftovers는 패턴을 실제로 펼쳐 존재하고 블랙리스트가 아닌 경로만 돌려준다.
// bundle 패턴(glob)을 먼저 처리해, 같은 경로가 이름 경로와 겹치면 높은 확신이 이긴다.
func gatherLeftovers(home, bundleID, appName string) []PathMatch {
	seen := map[string]bool{}
	var out []PathMatch
	take := func(specs []globSpec, literal bool) {
		for _, s := range specs {
			var found []string
			if literal {
				if Exists(s.pattern) {
					found = []string{s.pattern}
				}
			} else {
				found, _ = filepath.Glob(s.pattern)
			}
			for _, m := range found {
				if seen[m] || IsProtected(m) {
					continue
				}
				seen[m] = true
				out = append(out, PathMatch{Path: m, Confidence: s.confidence, Reason: s.reason})
			}
		}
	}
	take(bundleGlobs(home, bundleID), false)
	take(nameGlobs(home, appName), true)
	return out
}

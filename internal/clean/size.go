package clean

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SizePaths는 **용량 측정 전용** 경로 목록이다. 삭제하지 않는다.
//
// LeftoverPaths(삭제 계획과 동일한 규칙)만으로는 실제 디스크 점유를 크게 놓친다.
// 앱의 Bundle ID와 데이터 폴더 이름이 안 맞는 흔한 경우가 있기 때문이다:
//   - 벤더 폴더: Chrome 데이터는 ~/Library/Application Support/Google/Chrome (8GB+).
//     Bundle ID(com.google.Chrome)로도 앱 이름("Google Chrome")으로도 직접 안 잡힌다.
//   - 그룹 컨테이너: OrbStack 데이터는 Group Containers/HUAQ24HBR6.dev.orbstack (24GB).
//     Bundle ID가 dev.kdrag0n.MacVirt라 Bundle ID 매칭으로는 영영 못 찾는다.
//
// 이 확장 규칙들은 "이 앱 것이 맞을 가능성이 높다"는 추정이라 측정에만 쓴다.
// 삭제 계획(BuildPlan)은 벤더 공유 여부를 따로 판정하므로 여기 결과를 쓰지 않는다.
func SizePaths(home, bundleID, appName string) []PathMatch {
	out := LeftoverPaths(home, bundleID, appName)
	seen := map[string]bool{}
	for _, m := range out {
		seen[m.Path] = true
	}
	add := func(path, reason string) {
		if path == "" || seen[path] || IsProtected(path) {
			return
		}
		if _, err := os.Lstat(path); err != nil {
			return
		}
		// 이미 잡힌 경로의 하위면 중복 계산이 된다.
		for p := range seen {
			if strings.HasPrefix(path, p+string(filepath.Separator)) {
				return
			}
		}
		seen[path] = true
		out = append(out, PathMatch{Path: path, Confidence: ConfLow, Reason: reason})
	}

	for _, p := range vendorAppDirs(home, bundleID, appName) {
		add(p, "벤더 폴더 내 앱 데이터 (측정용)")
	}
	for _, p := range groupContainersByName(home, appName) {
		add(p, "그룹 컨테이너 (이름 기반 · 측정용)")
	}
	return out
}

// vendorAppDirs는 벤더 폴더 안의 이 앱 전용 하위 폴더들을 돌려준다.
// (예: ~/Library/Application Support/Google/Chrome). 벤더 공유 컴포넌트는 포함하지 않는다.
func vendorAppDirs(home, bundleID, appName string) []string {
	v, subdir, ok := lookupVendor(vendors(), bundleID, appName)
	if !ok || subdir == "" {
		return nil
	}
	var paths []string
	for _, root := range libraryRoots(home) {
		for _, folder := range v.Folders {
			paths = append(paths, filepath.Join(root, folder, subdir))
		}
	}
	return paths
}

// teamIDPrefix는 그룹 컨테이너 이름 앞의 개발자 팀 ID다(예: HUAQ24HBR6.dev.orbstack).
var teamIDPrefix = regexp.MustCompile(`^[A-Z0-9]{10}\.`)

// groupContainersByName은 팀 ID를 뗀 그룹 컨테이너 이름의 마지막 마디가 앱 이름과
// 같은 디렉토리를 찾는다(순수 매칭은 groupContainerMatchesApp — 테스트 대상).
func groupContainersByName(home, appName string) []string {
	dir := filepath.Join(home, "Library", "Group Containers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if groupContainerMatchesApp(e.Name(), appName) {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths
}

// groupContainerMatchesApp은 그룹 컨테이너 폴더명이 이 앱의 것인지 판정한다(순수 함수).
// 팀 ID 접두사를 떼고 마지막 마디를 앱 이름(공백 제거, 대소문자 무시)과 정확히 비교한다.
// 부분일치를 쓰지 않는 이유: "Mail"/"Home" 같은 짧은 이름이 무관한 컨테이너를 쓸어담는다.
func groupContainerMatchesApp(folder, appName string) bool {
	name := normalizeAppKey(appName)
	if name == "" {
		return false
	}
	stripped := teamIDPrefix.ReplaceAllString(folder, "")
	idx := strings.LastIndex(stripped, ".")
	if idx < 0 {
		return false // 마디가 하나뿐이면 Bundle ID 꼴이 아니다 — 이름 매칭 대상 아님
	}
	return normalizeAppKey(stripped[idx+1:]) == name
}

// normalizeAppKey는 비교용으로 공백/하이픈을 지우고 소문자화한다.
func normalizeAppKey(s string) string {
	s = strings.TrimSuffix(s, ".app")
	s = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s)
	return strings.ToLower(s)
}

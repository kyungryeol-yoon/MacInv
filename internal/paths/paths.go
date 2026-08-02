// Package paths는 macOS의 앱/데이터 경로를 찾고 판별하는 **저수준 유틸**이다.
//
// 레이어 규칙: paths는 아무 내부 패키지에도 위로 의존하지 않는다(exec만 사용).
// source(스캔)와 clean(계획 수립)이 모두 여기에 **아래로만** 의존한다 —
// 예전에는 source/apps.go가 clean을 import해 의존 방향이 뒤집혀 있었다.
//
// 여기 있는 것: 시스템 경로 블랙리스트, 잔여물 경로 매칭(glob/리터럴),
// 용량 측정용 경로 집합, 벤더 폴더 테이블(vendors.json), Bundle ID 유틸.
// 여기 없는 것: 삭제/업데이트 계획(Plan/Target/Execute) — 그건 clean의 몫이다.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// 확신 수준. 경로가 정말 이 앱 것인지에 대한 확신이다.
const (
	ConfHigh   = "high"   // Bundle ID 정확 일치
	ConfMedium = "medium" // 벤더 폴더 전체(그 벤더 앱이 이것뿐일 때)
	ConfLow    = "low"    // 이름/벤더 하위 폴더 기반 추정
)

// PathMatch는 후보 경로 하나와 그 확신 수준이다.
type PathMatch struct {
	Path       string
	Confidence string
	Reason     string
	// SizeKB는 측정된 용량이다(0 = 미측정 또는 부모 경로에 이미 포함됨).
	// 삭제 계획의 "회수 예상"을 내려고 clean.MeasureTrashSizes가 채운다.
	SizeKB int
}

// protectedPrefixes는 절대 접근 금지 경로다(하드코딩 블랙리스트).
var protectedPrefixes = []string{
	"/System",
	"/usr",
	"/bin",
	"/sbin",
	"/Library/Apple",
}

// IsProtected는 경로가 시스템 블랙리스트에 속하는지 판별한다(순수 함수 — 테스트 대상).
// 삭제 계획에 넣기 전에 반드시 통과해야 한다.
func IsProtected(path string) bool {
	clean := filepath.Clean(path)
	for _, p := range protectedPrefixes {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}

// IsAppleSystemApp은 Bundle ID로 애플 기본 앱을 판별한다(순수 함수 — 테스트 대상).
// Safari처럼 /Applications에 있어 경로 블랙리스트에 안 걸리는 시스템 앱을 보호한다.
func IsAppleSystemApp(bundleID string) bool {
	return strings.HasPrefix(bundleID, "com.apple.")
}

// Exists는 경로가 존재하는지 본다(심볼릭 링크 자체도 존재로 친다).
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// AppRoots는 GUI 앱을 찾을 디렉토리다(시스템 앱 디렉토리는 제외).
func AppRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{"/Applications", filepath.Join(home, "Applications"), "/Applications/Utilities"}
}

// LibraryRoots는 벤더 폴더가 존재할 수 있는 Library 하위 루트다.
func LibraryRoots(home string) []string {
	lib := filepath.Join(home, "Library")
	return []string{
		filepath.Join(lib, "Application Support"),
		filepath.Join(lib, "Caches"),
		filepath.Join(lib, "Logs"),
		lib, // ~/Library/<Vendor> (예: ~/Library/Google)
	}
}

// GlobHome은 home 기준 상대 glob 패턴을 펼친다.
func GlobHome(home, rel string) []string {
	matches, _ := filepath.Glob(filepath.Join(home, rel))
	return matches
}

// escapeGlobMeta는 glob 메타문자를 이스케이프한다(filepath.Match의 \ 규칙).
// 패턴에 끼워 넣는 가변 부분(Bundle ID 등)에만 적용한다 — 의도한 '*'는 그대로 둔다.
func escapeGlobMeta(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

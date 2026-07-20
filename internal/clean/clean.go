// Package clean은 삭제 플래너다. macinv의 안전 규칙(CLAUDE.md)을 코드로 강제한다:
//   - 기본은 dry-run: 계획만 만들고, 실제 삭제는 Execute를 명시적으로 호출할 때만.
//   - 삭제는 rm이 아니라 휴지통 이동(trash 또는 Finder).
//   - 시스템 경로(블랙리스트)는 어떤 경우에도 계획/삭제 대상에서 제외.
//   - 잔여물은 Bundle ID 정확 일치를 우선(높은 확신), 이름 기반은 낮은 확신으로 별도 표시.
package clean

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

// 확신 수준.
const (
	ConfHigh   = "high"   // Bundle ID 정확 일치
	ConfMedium = "medium" // 벤더 폴더 전체(그 벤더 앱이 이것뿐일 때)
	ConfLow    = "low"    // 이름/벤더 하위 폴더 기반 추정
)

// 타깃 종류.
const (
	KindApp         = "app"
	KindBrewFormula = "brew-formula"
	KindBrewCask    = "brew-cask"
)

// Plan은 <query>에 대한 실행 계획이다(삭제 또는 업데이트).
type Plan struct {
	Query   string
	Targets []Target
	Verb    string // "삭제"(기본) 또는 "업데이트" — 렌더 제목에 사용
	Icon    string // "🧹"(기본) 또는 "⬆️"
}

func (p *Plan) verb() string {
	if p.Verb == "" {
		return "삭제"
	}
	return p.Verb
}

func (p *Plan) icon() string {
	if p.Icon == "" {
		return "🧹"
	}
	return p.Icon
}

// Target은 삭제 대상 하나(앱/포뮬러/캐스크)와 그 부속물이다.
type Target struct {
	Kind          string
	Name          string
	BundleID      string
	Commands      []string    // 실행할 네이티브 삭제 명령(brew 등)
	CmdTimeoutSec int         // Commands 실행 타임아웃(0이면 기본 300초). 대형 다운로드가 있는 업그레이드만 상향.
	TrashPaths    []PathMatch // 휴지통으로 옮길 경로
	Excluded      []Exclusion // 일부러 제외한 경로(공유 컴포넌트 등) + 사유
	Warnings      []string
	Suggestions   []string
}

// PathMatch는 삭제 후보 경로 하나와 그 확신 수준이다.
type PathMatch struct {
	Path       string
	Confidence string
	Reason     string
}

// Exclusion은 계획에서 일부러 제외한 경로와 그 사유다(예: 타 벤더 앱이 공유 중).
type Exclusion struct {
	Path   string
	Reason string
}

// HasContent는 계획에 실제로 할 일이 있는지 여부다.
func (p *Plan) HasContent() bool {
	for _, t := range p.Targets {
		if len(t.Commands) > 0 || len(t.TrashPaths) > 0 {
			return true
		}
	}
	return false
}

// LowConfidenceCount는 계획 전체의 낮은 확신 경로 수를 센다.
func (p *Plan) LowConfidenceCount() int {
	n := 0
	for _, t := range p.Targets {
		for _, pm := range t.TrashPaths {
			if pm.Confidence == ConfLow {
				n++
			}
		}
	}
	return n
}

// WithoutLowConfidence는 낮은 확신 경로를 제거한 계획 사본을 돌려준다(실제 삭제 시 안전 기본값).
func (p *Plan) WithoutLowConfidence() *Plan {
	return p.WithSelectedLow(nil)
}

// WithSelectedLow는 high+medium 경로는 항상, 확신낮음 경로는 enabled[path]가 true인 것만 담은 사본을 돌려준다.
// enabled가 nil이면 확신낮음은 전부 제외(안전 기본값). 실행 직전 effective plan 생성에 쓴다.
func (p *Plan) WithSelectedLow(enabled map[string]bool) *Plan {
	out := &Plan{Query: p.Query, Verb: p.Verb, Icon: p.Icon}
	for _, t := range p.Targets {
		nt := t
		nt.TrashPaths = nil
		for _, pm := range t.TrashPaths {
			if pm.Confidence != ConfLow || enabled[pm.Path] {
				nt.TrashPaths = append(nt.TrashPaths, pm)
			}
		}
		out.Targets = append(out.Targets, nt)
	}
	return out
}

// BuildCombinedPlan은 여러 대상의 삭제 계획을 하나로 합친다(TUI 멀티선택용, 읽기 전용).
func BuildCombinedPlan(ctx context.Context, run *iexec.Runner, names []string) *Plan {
	combined := &Plan{Query: strings.Join(names, ", "), Verb: "삭제", Icon: "🧹"}
	for _, n := range names {
		p := BuildPlan(ctx, run, n)
		combined.Targets = append(combined.Targets, p.Targets...)
	}
	return combined
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

// globSpec은 잔여물 후보를 찾는 glob 패턴 하나다(존재 확인 전).
type globSpec struct {
	pattern    string
	confidence string
	reason     string
}

// bundleGlobs는 Bundle ID로 잔여물 glob 패턴을 만든다(순수 함수 — 테스트 대상).
// 접두사 매칭(<B>*)이라 헬퍼/파생 파일(com.vendor.app.helper.plist 등)까지 잡는다. 모두 높은 확신.
func bundleGlobs(home, bundleID string) []globSpec {
	if bundleID == "" {
		return nil
	}
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

// nameGlobs는 앱 이름 기반 잔여물 패턴을 만든다(순수 함수 — 테스트 대상). 낮은 확신.
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

// gatherLeftovers는 glob 패턴을 실제로 펼쳐 존재하고 블랙리스트가 아닌 경로만 돌려준다.
// bundle 패턴을 먼저 처리해, 같은 경로가 이름 패턴과 겹치면 높은 확신이 이긴다.
func gatherLeftovers(home, bundleID, appName string) []PathMatch {
	specs := append(bundleGlobs(home, bundleID), nameGlobs(home, appName)...)
	seen := map[string]bool{}
	var out []PathMatch
	for _, s := range specs {
		matches, _ := filepath.Glob(s.pattern)
		for _, m := range matches {
			if seen[m] || IsProtected(m) {
				continue
			}
			seen[m] = true
			out = append(out, PathMatch{Path: m, Confidence: s.confidence, Reason: s.reason})
		}
	}
	return out
}

// BuildPlan은 query에 해당하는 삭제 계획을 만든다(읽기 전용 — 아무것도 삭제하지 않는다).
func BuildPlan(ctx context.Context, run *iexec.Runner, query string) *Plan {
	home, _ := os.UserHomeDir()
	plan := &Plan{Query: query}

	caskMatched := false

	// 1) brew cask
	if brewInstalled(ctx, run, query, true) {
		caskMatched = true
		t := Target{Kind: KindBrewCask, Name: query}
		t.Commands = append(t.Commands, "brew uninstall --cask "+query)
		// 캐스크가 설치한 앱의 Bundle ID로 잔여 설정 파일도 정리.
		if app := findAppByName(query); app != "" {
			t.BundleID = bundleIDOf(ctx, run, app)
			addLeftovers(&t, home, t.BundleID, filepath.Base(strings.TrimSuffix(app, ".app")))
		}
		t.Suggestions = append(t.Suggestions, "삭제 후 남은 의존성 정리: brew autoremove")
		plan.Targets = append(plan.Targets, t)
	}

	// 2) brew formula
	if brewInstalled(ctx, run, query, false) {
		t := Target{Kind: KindBrewFormula, Name: query}
		t.Commands = append(t.Commands, "brew uninstall "+query)
		if deps := reverseDeps(ctx, run, query); len(deps) > 0 {
			t.Warnings = append(t.Warnings,
				"다음 설치된 패키지가 '"+query+"'에 의존합니다 → 삭제 시 문제 가능: "+strings.Join(deps, ", "))
		}
		t.Suggestions = append(t.Suggestions, "삭제 후 고아 의존성 정리: brew autoremove")
		plan.Targets = append(plan.Targets, t)
	}

	// 2.5) 패키지 매니저 전역 항목 (npm/pipx/cargo/go bin)
	appendPackageTargets(ctx, run, plan, query, home)

	// 3) 독립 GUI 앱 (brew cask로 이미 처리된 경우는 제외)
	if !caskMatched {
		apps, exact := findAppsByName(query)
		var installedIDs []string
		if len(apps) > 0 {
			installedIDs = installedAppBundleIDs(ctx, run) // 벤더 판단용(다른 벤더 앱 설치 여부)
		}
		for _, app := range apps {
			if IsProtected(app) {
				continue // 시스템 경로의 앱은 건드리지 않음
			}
			bundleID := bundleIDOf(ctx, run, app)
			if IsAppleSystemApp(bundleID) {
				continue // 애플 기본 앱(Safari 등)은 경로가 /Applications라도 보호
			}
			appName := strings.TrimSuffix(filepath.Base(app), ".app")
			t := Target{Kind: KindApp, Name: filepath.Base(app), BundleID: bundleID}
			if !exact {
				t.Warnings = append(t.Warnings,
					"이름 부분일치로 찾음('"+query+"' → '"+appName+"') — 대상이 맞는지 확인하세요")
			}
			// 앱 번들 자체를 휴지통으로.
			t.TrashPaths = append(t.TrashPaths, PathMatch{Path: app, Confidence: ConfHigh, Reason: "앱 번들"})
			addLeftovers(&t, home, bundleID, appName)
			// 벤더 폴더(Google/, Microsoft/ 등) 처리: 인벤토리 참조해 하위만/전체 결정.
			augmentVendor(&t, home, bundleID, appName, installedIDs)
			// 다른 위치의 동일 Bundle ID 앱 사본(mdfind)도 후보로.
			for _, other := range mdfindByBundleID(ctx, run, bundleID) {
				if other != app && !IsProtected(other) && strings.HasSuffix(other, ".app") {
					t.TrashPaths = append(t.TrashPaths, PathMatch{Path: other, Confidence: ConfHigh, Reason: "동일 Bundle ID 앱 사본"})
				}
			}
			plan.Targets = append(plan.Targets, t)
		}
	}

	return plan
}

// augmentVendor는 벤더 폴더(Google/ 등)를 인벤토리 기준으로 처리한다:
//   - 같은 벤더의 다른 앱이 설치돼 있으면: <Vendor>/<App> 하위만 [확신낮음]으로 추가하고,
//     공유 컴포넌트(Keystone 등)는 "타 앱 사용 중" 사유로 제외 목록에 표시.
//   - 이 벤더 앱이 이것뿐이면: 벤더 폴더 전체를 [확신중간]으로 추가(공유 컴포넌트 포함).
func augmentVendor(t *Target, home, bundleID, appName string, installedIDs []string) {
	v, appSubdir, ok := lookupVendor(vendors(), bundleID, appName)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, p := range t.TrashPaths {
		seen[p.Path] = true
	}

	others := countVendorApps(v, installedIDs, bundleID)
	roots := libraryRoots(home)

	if others > 0 {
		// 벤더 폴더는 공유 중 → 앱 전용 하위 폴더만.
		for _, folder := range v.Folders {
			for _, root := range roots {
				sub := filepath.Join(root, folder, appSubdir)
				if seen[sub] || IsProtected(sub) || !exists(sub) {
					continue
				}
				seen[sub] = true
				t.TrashPaths = append(t.TrashPaths, PathMatch{
					Path: sub, Confidence: ConfLow,
					Reason: v.Name + " 폴더 하위 (앱 전용 · 확신 낮음)",
				})
			}
		}
		// 공유 컴포넌트는 보존하고 제외 목록에 표시.
		for _, sc := range v.SharedComponents {
			for _, g := range sc.Globs {
				for _, m := range globHome(home, g) {
					if IsProtected(m) {
						continue
					}
					t.Excluded = append(t.Excluded, Exclusion{
						Path:   m,
						Reason: "타 " + v.Name + " 앱이 사용 중 — 보존 (" + sc.Name + ")",
					})
				}
			}
		}
		return
	}

	// 이 벤더 앱이 이것뿐 → 벤더 폴더 전체를 [확신중간].
	for _, folder := range v.Folders {
		for _, root := range roots {
			dir := filepath.Join(root, folder)
			if seen[dir] || IsProtected(dir) || !exists(dir) {
				continue
			}
			seen[dir] = true
			t.TrashPaths = append(t.TrashPaths, PathMatch{
				Path: dir, Confidence: ConfMedium,
				Reason: v.Name + " 폴더 전체 (이 벤더 앱은 이것뿐 · 확신 중간)",
			})
		}
	}
	// 공유 컴포넌트(Keystone 등)도 이제 고아이므로 함께 정리(폴더 밖 LaunchAgents 등).
	// 단, 이미 담긴 벤더 폴더 아래에 있는 경로는 중복이므로 건너뛴다.
	for _, sc := range v.SharedComponents {
		for _, g := range sc.Globs {
			for _, m := range globHome(home, g) {
				if seen[m] || IsProtected(m) || coveredByExisting(m, t.TrashPaths) {
					continue
				}
				seen[m] = true
				t.TrashPaths = append(t.TrashPaths, PathMatch{
					Path: m, Confidence: ConfMedium,
					Reason: v.Name + " 공유 컴포넌트 (이 벤더 앱은 이것뿐 · " + sc.Name + ")",
				})
			}
		}
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// coveredByExisting은 path가 이미 담긴 어떤 경로와 같거나 그 하위인지 본다(중복 삭제 방지).
func coveredByExisting(path string, existing []PathMatch) bool {
	for _, p := range existing {
		if path == p.Path || strings.HasPrefix(path, p.Path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// globHome은 home 기준 상대 glob 패턴을 펼친다.
func globHome(home, rel string) []string {
	matches, _ := filepath.Glob(filepath.Join(home, rel))
	return matches
}

// installedAppBundleIDs는 앱 디렉토리의 *.app들의 Bundle ID를 모은다(벤더 판단용).
func installedAppBundleIDs(ctx context.Context, run *iexec.Runner) []string {
	var ids []string
	for _, root := range appRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			if id := bundleIDOf(ctx, run, filepath.Join(root, e.Name())); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// addLeftovers는 존재하고 블랙리스트가 아닌 잔여물만 타깃에 더한다(이미 담긴 경로는 건너뜀).
func addLeftovers(t *Target, home, bundleID, appName string) {
	seen := map[string]bool{}
	for _, p := range t.TrashPaths {
		seen[p.Path] = true
	}
	for _, pm := range gatherLeftovers(home, bundleID, appName) {
		if seen[pm.Path] {
			continue
		}
		seen[pm.Path] = true
		t.TrashPaths = append(t.TrashPaths, pm)
	}
}

// ---- 시스템 조회 헬퍼 (읽기 전용) ----------------------------------------

func brewInstalled(ctx context.Context, run *iexec.Runner, name string, cask bool) bool {
	args := []string{"list", "--versions"}
	if cask {
		args = append(args, "--cask")
	} else {
		args = append(args, "--formula")
	}
	args = append(args, name)
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 30, OKCodes: []int{0, 1}}, "brew", args...)
	return err == nil && strings.TrimSpace(out) != ""
}

func reverseDeps(ctx context.Context, run *iexec.Runner, name string) []string {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60, OKCodes: []int{0, 1}}, "brew", "uses", "--installed", name)
	if err != nil {
		return nil
	}
	var deps []string
	for _, f := range strings.Fields(out) {
		if f != "" {
			deps = append(deps, f)
		}
	}
	return deps
}

func bundleIDOf(ctx context.Context, run *iexec.Runner, appPath string) string {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 15}, "mdls", "-raw", "-name", "kMDItemCFBundleIdentifier", appPath)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(out)
	if v == "" || v == "(null)" {
		return ""
	}
	return v
}

func mdfindByBundleID(ctx context.Context, run *iexec.Runner, bundleID string) []string {
	if bundleID == "" {
		return nil
	}
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 20},
		"mdfind", "kMDItemCFBundleIdentifier == '"+bundleID+"'")
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// appRoots는 GUI 앱을 찾을 디렉토리다(시스템 앱 디렉토리는 제외).
func appRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{"/Applications", filepath.Join(home, "Applications"), "/Applications/Utilities"}
}

// appEntry는 앱 파일명과 전체 경로 쌍이다(매칭 로직 테스트 분리용).
type appEntry struct {
	name string // "Google Chrome.app"
	path string
}

// matchApps는 정확 일치 우선으로 앱을 고른다(순수 함수 — 테스트 대상).
// 대소문자 무시 정확 일치(stem == query)가 있으면 그것만 돌려주고 exact=true.
// 없으면 부분일치 전부를 돌려주고 exact=false — 호출자는 경고를 붙여야 한다.
func matchApps(entries []appEntry, query string) (matched []appEntry, exact bool) {
	q := strings.ToLower(query)
	var exactM, partial []appEntry
	for _, e := range entries {
		if !strings.HasSuffix(e.name, ".app") {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(e.name, ".app"))
		switch {
		case stem == q:
			exactM = append(exactM, e)
		case strings.Contains(stem, q):
			partial = append(partial, e)
		}
	}
	if len(exactM) > 0 {
		return exactM, true
	}
	return partial, false
}

// findAppsByName은 이름이 query에 맞는 *.app 경로를 찾는다(정확 일치 우선, 없으면 부분일치).
func findAppsByName(query string) (paths []string, exact bool) {
	var entries []appEntry
	for _, root := range appRoots() {
		dirents, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range dirents {
			entries = append(entries, appEntry{name: e.Name(), path: filepath.Join(root, e.Name())})
		}
	}
	matched, exact := matchApps(entries, query)
	for _, m := range matched {
		paths = append(paths, m.path)
	}
	return paths, exact
}

// findAppByName은 findAppsByName의 첫 결과를 돌려준다(캐스크 잔여물 조회용).
func findAppByName(query string) string {
	if apps, _ := findAppsByName(query); len(apps) > 0 {
		return apps[0]
	}
	return ""
}

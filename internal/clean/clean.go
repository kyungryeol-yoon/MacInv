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
	"sync"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/paths"
)

// 경로 판별·매칭은 저수준 paths 패키지가 담당한다(source도 같은 것을 쓴다).
// 여기서는 별칭으로 노출해 clean의 공개 API(clean.PathMatch/ConfHigh …)를 유지한다.
type (
	// PathMatch는 삭제 후보 경로 하나와 그 확신 수준이다.
	PathMatch = paths.PathMatch
)

// 확신 수준.
const (
	ConfHigh   = paths.ConfHigh   // Bundle ID 정확 일치
	ConfMedium = paths.ConfMedium // 벤더 폴더 전체(그 벤더 앱이 이것뿐일 때)
	ConfLow    = paths.ConfLow    // 이름/벤더 하위 폴더 기반 추정
)

// 안전 판정·경로 조회는 paths의 것을 그대로 쓴다(규칙이 두 벌이 되면 안 된다).
var (
	// IsProtected는 경로가 시스템 블랙리스트에 속하는지 판별한다.
	IsProtected = paths.IsProtected
	// IsAppleSystemApp은 Bundle ID로 애플 기본 앱을 판별한다.
	IsAppleSystemApp = paths.IsAppleSystemApp
	// BundleIDFromPlist는 Info.plist에서 CFBundleIdentifier를 직접 읽는다(mdls 폴백).
	BundleIDFromPlist = paths.BundleIDFromPlist
	// LeftoverPaths는 앱이 남긴 데이터 경로다(삭제 계획과 동일한 매칭 규칙).
	LeftoverPaths = paths.LeftoverPaths
	// SizePaths는 용량 측정 전용 경로다(삭제에는 쓰지 않는다).
	SizePaths = paths.SizePaths
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
	// RequireConfirm은 승인 뒤 한 번 더(y/N) 물어야 하는 계획이다.
	// GUI 앱 삭제처럼 되돌리기 어려운 것에 붙인다(disk의 재다운로드 비싼 항목 등).
	RequireConfirm bool
	// ExtraReclaimKB는 휴지통 경로 **밖에서** 회수될 용량이다.
	// 네이티브 명령(brew cleanup, npm cache clean …)이 지우는 몫은 TrashPaths에 없어
	// 이걸 안 더하면 회수 예상이 실제보다 훨씬 작게 나온다(disk 계획에서 사용).
	ExtraReclaimKB int
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
	Kind     string
	Name     string
	BundleID string
	// Argv는 실행할 네이티브 명령들이다. 문자열이 아니라 인자 배열로 담는다 —
	// 문자열로 두면 실행 직전에 공백으로 다시 쪼개야 해서 공백이 든 인자가 깨진다.
	// 표시용 한 줄은 ShellQuote가 렌더 시점에 만든다.
	Argv          [][]string
	CmdTimeoutSec int         // Argv 실행 타임아웃(0이면 기본 300초). 대형 다운로드가 있는 업그레이드만 상향.
	TrashPaths    []PathMatch // 휴지통으로 옮길 경로
	NeedsAdmin    []PathMatch // 쓰기 권한이 없어 자동 삭제하지 않는 경로(수동 처리 안내)
	Excluded      []Exclusion // 일부러 제외한 경로(공유 컴포넌트 등) + 사유
	Warnings      []string
	Suggestions   []string
}

// Exclusion은 계획에서 일부러 제외한 경로와 그 사유다(예: 타 벤더 앱이 공유 중).
type Exclusion struct {
	Path   string
	Reason string
}

// HasContent는 계획에 실제로 할 일이 있는지 여부다.
func (p *Plan) HasContent() bool {
	for _, t := range p.Targets {
		if len(t.Argv) > 0 || len(t.TrashPaths) > 0 {
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
	out := &Plan{Query: p.Query, Verb: p.Verb, Icon: p.Icon,
		RequireConfirm: p.RequireConfirm, ExtraReclaimKB: p.ExtraReclaimKB}
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

// ---- 접두사 과매칭 방어 (다른 앱 소유 경로 보존) ---------------------------
//
// bundleGlobs는 접두사 매칭이라 com.google.Chrome 계획에 com.google.Chrome.canary가
// [확실]로 딸려 들어온다. [확실]은 --apply 기본 삭제 대상이라 방어선이 없으므로,
// 경로에서 뽑은 파생 ID가 **설치된 다른 앱의 Bundle ID에 귀속**되면 계획에서 뺀다.
// 반대로 com.google.Chrome.helper처럼 어느 앱도 소유하지 않는 파생물은 그대로 남긴다.

// installedApp은 설치된 앱의 Bundle ID와 표시 이름이다.
type installedApp struct {
	ID   string
	Name string
}

// ownerOfID는 파생 ID를 소유하는 설치 앱을 찾는다(가장 긴 = 가장 구체적인 ID가 이긴다).
// 가장 긴 것을 고르는 이유: Chrome과 Chrome.canary가 둘 다 설치된 상태에서 canary를 지울 때
// canary 파생물이 Chrome 소유로 오판되면 안 되기 때문이다.
func ownerOfID(candidate string, installed []installedApp) (installedApp, bool) {
	var best installedApp
	for _, a := range installed {
		if a.ID == "" {
			continue
		}
		if candidate == a.ID || strings.HasPrefix(candidate, a.ID+".") {
			if len(a.ID) > len(best.ID) {
				best = a
			}
		}
	}
	return best, best.ID != ""
}

// excludeOtherAppIDs는 다른 설치 앱이 소유하는 경로를 계획에서 빼고 보존 사유를 만든다
// (순수 함수 — 테이블 테스트 대상).
func excludeOtherAppIDs(matches []PathMatch, selfID string, installed []installedApp) (kept []PathMatch, excluded []Exclusion) {
	if selfID == "" {
		return matches, nil
	}
	for _, m := range matches {
		cand := paths.DerivedBundleID(m.Path)
		if owner, ok := ownerOfID(cand, installed); ok && owner.ID != selfID {
			name := owner.Name
			if name == "" {
				name = owner.ID
			}
			excluded = append(excluded, Exclusion{
				Path:   m.Path,
				Reason: "타 앱(" + name + ") 소유 — 보존",
			})
			continue
		}
		kept = append(kept, m)
	}
	return kept, excluded
}

// filterOtherAppOwned는 excludeOtherAppIDs를 적용하되, 검사할 파생 ID가 하나도 없으면
// 설치 목록 조회(앱마다 mdls — 비쌈)를 아예 하지 않는다.
func filterOtherAppOwned(matches []PathMatch, selfID string, installed func() []installedApp) ([]PathMatch, []Exclusion) {
	if selfID == "" || installed == nil {
		return matches, nil
	}
	need := false
	for _, m := range matches {
		if id := paths.DerivedBundleID(m.Path); id != "" && id != selfID {
			need = true
			break
		}
	}
	if !need {
		return matches, nil
	}
	return excludeOtherAppIDs(matches, selfID, installed())
}

// BuildPlan은 query에 해당하는 삭제 계획을 만든다(읽기 전용 — 아무것도 삭제하지 않는다).
func BuildPlan(ctx context.Context, run *iexec.Runner, query string) *Plan {
	home, _ := os.UserHomeDir()
	plan := &Plan{Query: query}

	// 독립적인 소스 프로브(brew cask/formula)는 병렬로 미리 조회한다 — 계획 대기시간 단축.
	var (
		probeWg           sync.WaitGroup
		caskOK, formulaOK bool
	)
	probeWg.Add(2)
	go func() { defer probeWg.Done(); caskOK = brewInstalled(ctx, run, query, true) }()
	go func() { defer probeWg.Done(); formulaOK = brewInstalled(ctx, run, query, false) }()
	probeWg.Wait()

	// 설치된 앱의 Bundle ID 수집은 비싸므로(앱마다 mdls) 실제로 필요할 때 1회만 수행한다.
	// 벤더 판단(augmentVendor)과 접두사 과매칭 방어(filterOtherAppOwned)가 공유한다.
	installed := sync.OnceValue(func() []installedApp { return installedAppList(ctx, run) })

	caskMatched := false

	// 1) brew cask
	if caskOK {
		caskMatched = true
		t := Target{Kind: KindBrewCask, Name: query}
		t.Argv = append(t.Argv, []string{"brew", "uninstall", "--cask", query})
		// 캐스크가 설치한 앱의 Bundle ID로 잔여 설정 파일도 정리.
		if app := findAppByName(query); app != "" {
			t.BundleID = bundleIDOf(ctx, run, app)
			addLeftovers(&t, home, t.BundleID, filepath.Base(strings.TrimSuffix(app, ".app")), installed)
		}
		t.Suggestions = append(t.Suggestions, "삭제 후 남은 의존성 정리: brew autoremove")
		plan.Targets = append(plan.Targets, t)
	}

	// 2) brew formula
	if formulaOK {
		t := Target{Kind: KindBrewFormula, Name: query}
		t.Argv = append(t.Argv, []string{"brew", "uninstall", query})
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
			addLeftovers(&t, home, bundleID, appName, installed)
			// 벤더 폴더(Google/, Microsoft/ 등) 처리: 인벤토리 참조해 하위만/전체 결정.
			augmentVendor(&t, home, bundleID, appName, func() []string { return bundleIDsOf(installed()) })
			// 다른 위치의 동일 Bundle ID 앱 사본(mdfind)도 후보로.
			for _, other := range mdfindByBundleID(ctx, run, bundleID) {
				if other != app && !IsProtected(other) && strings.HasSuffix(other, ".app") {
					t.TrashPaths = append(t.TrashPaths, PathMatch{Path: other, Confidence: ConfHigh, Reason: "동일 Bundle ID 앱 사본"})
				}
			}
			plan.Targets = append(plan.Targets, t)
		}
	}

	// 권한이 없어 어차피 실패할 경로를 먼저 갈라내고(회수 예상에도 넣지 않는다),
	// 남은 경로만 재서 "회수 예상"을 낸다(앱 목록의 표시 용량과 다를 수 있다).
	markNeedsAdmin(plan)
	MeasureTrashSizes(ctx, run, plan)
	return plan
}

// augmentVendor는 벤더 폴더(Google/ 등)를 인벤토리 기준으로 처리한다:
//   - 같은 벤더의 다른 앱이 설치돼 있으면: <Vendor>/<App> 하위만 [확신낮음]으로 추가하고,
//     공유 컴포넌트(Keystone 등)는 "타 앱 사용 중" 사유로 제외 목록에 표시.
//   - 이 벤더 앱이 이것뿐이면: 벤더 폴더 전체를 [확신중간]으로 추가(공유 컴포넌트 포함).
//
// installedIDs는 지연 공급자다 — 벤더 매칭이 없으면 비싼 Bundle ID 수집을 아예 안 한다.
func augmentVendor(t *Target, home, bundleID, appName string, installedIDs func() []string) {
	v, appSubdir, ok := paths.LookupVendor(paths.Vendors(), bundleID, appName)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, p := range t.TrashPaths {
		seen[p.Path] = true
	}

	others := paths.CountVendorApps(v, installedIDs(), bundleID)
	roots := paths.LibraryRoots(home)

	if others > 0 {
		// 벤더 폴더는 공유 중 → 앱 전용 하위 폴더만.
		for _, folder := range v.Folders {
			for _, root := range roots {
				sub := filepath.Join(root, folder, appSubdir)
				if seen[sub] || IsProtected(sub) || !paths.Exists(sub) {
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
				for _, m := range paths.GlobHome(home, g) {
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
			if seen[dir] || IsProtected(dir) || !paths.Exists(dir) {
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
			for _, m := range paths.GlobHome(home, g) {
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

// coveredByExisting은 path가 이미 담긴 어떤 경로와 같거나 그 하위인지 본다(중복 삭제 방지).
func coveredByExisting(path string, existing []PathMatch) bool {
	for _, p := range existing {
		if path == p.Path || strings.HasPrefix(path, p.Path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// installedAppList는 앱 디렉토리의 *.app들의 Bundle ID + 이름을 모은다
// (벤더 판단 및 접두사 과매칭 방어용). 앱마다 mdls 1회라 병렬(worker 8)로 조회한다.
func installedAppList(ctx context.Context, run *iexec.Runner) []installedApp {
	var appPaths []string
	for _, root := range paths.AppRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				appPaths = append(appPaths, filepath.Join(root, e.Name()))
			}
		}
	}

	apps := make([]installedApp, len(appPaths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, p := range appPaths {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer wg.Done()
			defer func() { <-sem }()
			apps[i] = installedApp{
				ID:   bundleIDOf(ctx, run, p),
				Name: strings.TrimSuffix(filepath.Base(p), ".app"),
			}
		}(i, p)
	}
	wg.Wait()

	out := apps[:0]
	for _, a := range apps {
		if a.ID != "" {
			out = append(out, a)
		}
	}
	return out
}

// bundleIDsOf는 설치 앱 목록에서 Bundle ID만 뽑는다(벤더/고아 판정은 ID만 쓴다).
func bundleIDsOf(apps []installedApp) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}

// addLeftovers는 존재하고 블랙리스트가 아닌 잔여물만 타깃에 더한다(이미 담긴 경로는 건너뜀).
// 접두사 glob이 삼킨 "다른 설치 앱 소유" 경로는 삭제하지 않고 보존 목록(Excluded)으로 옮긴다.
func addLeftovers(t *Target, home, bundleID, appName string, installed func() []installedApp) {
	matches, excluded := filterOtherAppOwned(paths.LeftoverPaths(home, bundleID, appName), bundleID, installed)
	t.Excluded = append(t.Excluded, excluded...)
	seen := map[string]bool{}
	for _, p := range t.TrashPaths {
		seen[p.Path] = true
	}
	for _, pm := range matches {
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
	if err == nil {
		if v := strings.TrimSpace(out); v != "" && v != "(null)" {
			return v
		}
	}
	// Spotlight가 인덱싱하지 않은 앱이 있다(실측: Notion → mdls가 (null)).
	// Bundle ID를 못 구하면 잔여물을 이름 기반으로만 찾게 되어 notion.id.* 같은
	// 파일을 통째로 놓치므로, Info.plist를 직접 읽어 폴백한다.
	return BundleIDFromPlist(ctx, run, appPath)
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
func findAppsByName(query string) (found []string, exact bool) {
	var entries []appEntry
	for _, root := range paths.AppRoots() {
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
		found = append(found, m.path)
	}
	return found, exact
}

// findAppByName은 findAppsByName의 첫 결과를 돌려준다(캐스크 잔여물 조회용).
func findAppByName(query string) string {
	if apps, _ := findAppsByName(query); len(apps) > 0 {
		return apps[0]
	}
	return ""
}

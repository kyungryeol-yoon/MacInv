package clean

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
	"github.com/kyungryeol-yoon/MacInv/internal/paths"
)

// KindOrphan은 고아 잔여물(지워진 앱이 남긴 데이터) 타깃이다.
const KindOrphan = "orphan"

// orphanLoc은 고아 스캔 위치 하나다. strip이 비어있지 않으면 그 접미사를 가진 항목만 대상이고
// ID 추출 시 접미사를 벗긴다. ~/Library만 스캔한다(/Library는 다른 사용자/서비스 소유 가능성으로 제외).
type orphanLoc struct {
	rel    string
	strip  string
	reason string
}

var orphanLocs = []orphanLoc{
	{"Preferences", ".plist", "환경설정"},
	{"Preferences/ByHost", ".plist", "환경설정 ByHost"},
	{"Caches", "", "캐시"},
	{"HTTPStorages", "", "HTTP 저장소"},
	{"WebKit", "", "WebKit 데이터"},
	{"Containers", "", "샌드박스 컨테이너"},
	{"Application Scripts", "", "앱 스크립트"},
	{"Saved Application State", ".savedState", "저장된 상태"},
	{"Application Support", "", "앱 지원 데이터"},
	{"Logs", "", "로그"},
	{"LaunchAgents", ".plist", "LaunchAgent"},
	{"Cookies", ".binarycookies", "쿠키"},
	{"Group Containers", "", "그룹 컨테이너"},
}

// bundleIDShaped는 reverse-DNS 형태(점 2개 이상, 예: com.vendor.app)인지 본다.
// "Google", "Notion" 같은 이름 폴더는 귀속을 확신할 수 없으므로 고아 후보에서 아예 제외한다.
var bundleIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(\.[A-Za-z0-9_-]+){2,}$`)

// extractOrphanID는 항목 이름에서 Bundle ID를 추출한다(순수 함수 — 테스트 대상).
// strip 접미사가 요구되는데 없으면 ""(대상 아님). group. 접두사와 ByHost UUID 접미사를 정규화한다.
func extractOrphanID(name, strip string) string {
	if strip != "" {
		if !strings.HasSuffix(name, strip) {
			return ""
		}
		name = strings.TrimSuffix(name, strip)
	}
	name = paths.StripHostUUID(name)
	name = strings.TrimPrefix(name, "group.")
	return name
}

// orphanProtectedPrefixes는 고아 판정에서 무조건 보존하는 알려진 시스템/공유 ID 접두사다.
//   - is.workflow.       : Apple 단축어(Shortcuts) — 시스템 앱이라 /Applications 스캔에 없음
//   - org.sparkle-project.: Sparkle — 설치된 앱들이 임베드해 쓰는 공유 업데이트 프레임워크
var orphanProtectedPrefixes = []string{
	"is.workflow.",
	"org.sparkle-project.",
}

// orphanProtected는 구조적으로 보존해야 하는 ID인지 본다(순수 함수 — 테스트 대상).
func orphanProtected(id string) bool {
	// com.apple.이 어디에든 포함되면 애플 데이터(팀ID·systemgroup 접두사 포함)로 보고 보존.
	if strings.Contains(id, "com.apple.") {
		return true
	}
	if strings.HasPrefix(id, "systemgroup.") {
		return true
	}
	if paths.HasTeamIDPrefix(id) {
		return true
	}
	for _, pfx := range orphanProtectedPrefixes {
		if strings.HasPrefix(id, pfx) {
			return true
		}
	}
	return false
}

// classifyOrphan은 id가 고아(어느 설치 앱에도 속하지 않음)인지 판정한다(순수 함수 — 테스트 대상).
// 보수적 원칙: 애매하면 고아가 아니라고 본다.
//   - Bundle ID 형태가 아니면 제외
//   - 애플/시스템/공유 프레임워크(orphanProtected)는 무조건 보존
//   - 설치 ID와 양방향 접두사 일치(파생물 포함)면 보존
//   - 벤더 공유: id가 벤더 접두사에 맞고 그 벤더 앱이 하나라도 설치돼 있으면 보존
func classifyOrphan(id string, installed []string, vs []paths.Vendor) bool {
	if !bundleIDRe.MatchString(id) {
		return false
	}
	if orphanProtected(id) {
		return false
	}
	for _, inst := range installed {
		if inst == "" {
			continue
		}
		if strings.HasPrefix(id, inst) || strings.HasPrefix(inst, id) {
			return false
		}
	}
	for _, v := range vs {
		idInVendor := false
		for _, pfx := range v.BundlePrefixes {
			if strings.HasPrefix(id, pfx) {
				idInVendor = true
				break
			}
		}
		if !idInVendor {
			continue
		}
		for _, inst := range installed {
			for _, pfx := range v.BundlePrefixes {
				if strings.HasPrefix(inst, pfx) {
					return false // 같은 벤더 앱이 살아있음 → 공유 컴포넌트일 수 있어 보존
				}
			}
		}
	}
	return true
}

// BuildOrphansPlan은 ~/Library를 역스캔해 설치 목록에 없는 Bundle ID의 잔여물 계획을 만든다(읽기 전용).
// 확신도는 전부 [확신중간] — "설치 목록에 없음"은 추론이다(앱 없는 CLI가 plist를 쓰는 경우 등).
func BuildOrphansPlan(ctx context.Context, run *iexec.Runner) *Plan {
	home, _ := os.UserHomeDir()
	plan := &Plan{Query: "고아 잔여물 (설치 목록에 없는 Bundle ID)", Verb: "정리", Icon: "🧽"}

	installed := bundleIDsOf(installedAppList(ctx, run))
	vs := paths.Vendors()
	lib := filepath.Join(home, "Library")

	byID := map[string][]PathMatch{}
	for _, loc := range orphanLocs {
		dir := filepath.Join(lib, loc.rel)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			id := extractOrphanID(e.Name(), loc.strip)
			if id == "" || !classifyOrphan(id, installed, vs) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if IsProtected(p) {
				continue
			}
			byID[id] = append(byID[id], PathMatch{
				Path: p, Confidence: ConfMedium,
				Reason: loc.reason + " (설치 목록에 없는 ID · 확신 중간)",
			})
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindOrphan, Name: id, BundleID: id,
			TrashPaths: byID[id],
		})
	}
	markNeedsAdmin(plan)
	MeasureTrashSizes(ctx, run, plan)
	return plan
}

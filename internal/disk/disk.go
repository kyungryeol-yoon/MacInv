// Package disk는 "왜 디스크가 꽉 찼는지"를 알려주는 커맨드의 로직이다.
//
// 동기: 시스템 설정의 "문서"가 57GB인데 Finder로는 11GB만 보였다. 차이는 ~/.ollama 모델과
// 각종 개발 캐시였다 — Finder도 시스템 설정도 그걸 이름 지어 보여주지 않는다.
//
// 규약(CLAUDE.md):
//   - 판정·집계는 순수 함수, 외부 명령은 exec.Runner 경유.
//   - 정리 실행은 새로 만들지 않고 clean.Plan/Execute/RenderPlanText를 재사용한다.
//     안전 규칙(dry-run 기본, rm 금지, 블랙리스트, 확인 절차)이 두 벌이 되면 안 된다.
//   - 알려진 위치는 data/locations.json에 데이터로 둔다(vendors.json과 같은 방식) —
//     새 캐시 위치 추가에 코드 수정이 필요 없어야 한다.
package disk

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kyungryeol-yoon/macinv/internal/paths"
)

//go:embed data/locations.json
var locationsJSON []byte

// 등급 — "지워도 되는가"가 아니라 "지우면 무엇을 잃는가"를 나타낸다.
const (
	TierSafe      = "safe"      // 재생성됨 (캐시) — 잃는 것은 다음 실행의 속도뿐
	TierArtifact  = "artifact"  // 빌드 산출물 — 다시 빌드하면 되지만 시간이 든다
	TierExpensive = "expensive" // 재다운로드가 비쌈 (모델·이미지) — 기본 정리 대상에서 제외
	TierNever     = "never"     // 소스·사용자 데이터 — 어떤 경우에도 정리하지 않는다
)

// TierLabel은 등급의 한국어 라벨이다(CLI/TUI 공용).
func TierLabel(tier string) string {
	switch tier {
	case TierSafe:
		return "[안전]"
	case TierArtifact:
		return "[빌드산출물]"
	case TierExpensive:
		return "[재다운로드비쌈]"
	case TierNever:
		return "[정리안함]"
	default:
		return "[" + tier + "]"
	}
}

// Cleanable은 등급이 기본 정리 대상인지 본다(순수 함수 — 테스트 대상).
// expensive는 --include-expensive가 있을 때만, never는 어떤 경우에도 포함하지 않는다
// (clean의 --include-low와 같은 패턴).
func Cleanable(tier string, includeExpensive bool) bool {
	switch tier {
	case TierSafe, TierArtifact:
		return true
	case TierExpensive:
		return includeExpensive
	default:
		return false
	}
}

// Location은 알려진 디스크 사용처 하나다(data/locations.json 스키마).
type Location struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Globs []string `json:"globs"` // ~ 로 시작하면 홈 기준
	// Tool은 이 위치를 만든 도구다. 인벤토리에 없으면 [고아 캐시]로 표시한다.
	Tool string `json:"tool"`
	// OwnerApps는 CLI 도구가 없어도 이 캐시를 쓰는 **GUI 앱**들이다.
	// 예: gradle CLI를 안 깔아도 Android Studio는 ~/.gradle을 쓴다. 이게 없으면
	// "gradle 미설치 + Android Studio 설치" 상태에서 멀쩡한 캐시를 고아로 오탐한다.
	OwnerApps []string `json:"owner_apps,omitempty"`
	Tier      string   `json:"tier"`
	// Cleanup은 정리에 쓸 네이티브 명령이다(rm 아님). 비어 있으면 리포트 전용.
	Cleanup [][]string `json:"cleanup,omitempty"`
	// Trash는 네이티브 명령이 없어 휴지통 이동으로 정리하는 위치다(DerivedData 등).
	Trash bool `json:"trash,omitempty"`
	// GroupBy는 와일드카드 위치를 무엇 단위로 쪼개 보여줄지다("" = 쪼개지 않음).
	// 지금은 "app-support-dir"만 지원한다: ~/Library/Application Support/<앱>/... 의 <앱> 단위.
	// 한 항목에 66GB라고만 뜨면 어느 앱을 손봐야 할지 판단할 수 없어서 필요하다.
	GroupBy string `json:"group_by,omitempty"`
	Note    string `json:"note,omitempty"`
}

// GroupBy 값.
const GroupByAppSupportDir = "app-support-dir"

type locationTable struct {
	Locations []Location `json:"locations"`
}

var loadOnce = sync.OnceValue(func() []Location {
	var t locationTable
	_ = json.Unmarshal(locationsJSON, &t) // 임베드된 데이터라 실패할 일 없음
	return t.Locations
})

// Locations는 임베드된 알려진 위치 목록이다.
func Locations() []Location { return loadOnce() }

// ExpandGlob은 ~ 접두사를 홈으로 바꾼 뒤 glob을 펼친다(순수 함수 — home을 주입해 테스트).
// 존재하지 않는 경로는 결과에 없다.
func ExpandGlob(home, pattern string) []string {
	if pattern == "~" {
		pattern = home
	} else if strings.HasPrefix(pattern, "~/") {
		pattern = filepath.Join(home, pattern[2:])
	}
	matches, _ := filepath.Glob(pattern)
	sort.Strings(matches)
	return matches
}

// ResolvePaths는 위치들의 glob을 실제 경로로 펼친다(순수 함수에 가깝다 — 파일 존재만 본다).
// 중복 계산을 막으려고 **전체 위치를 가로질러** 부모/자식이 겹치는 경로를 한 번만 남긴다
// (paths.DedupContained 재사용 — 용량 도구에서 이중 계산은 신뢰도를 바로 깎는다).
// 앞선 위치가 우선권을 갖는다(locations.json의 나열 순서).
func ResolvePaths(home string, locs []Location) map[string][]string {
	var all []paths.PathMatch
	owner := map[string]string{} // 경로 → 위치 ID
	for _, l := range locs {
		for _, g := range l.Globs {
			for _, m := range ExpandGlob(home, g) {
				if paths.IsProtected(m) || HasAppleComponent(m) {
					continue
				}
				if _, dup := owner[m]; dup {
					continue
				}
				owner[m] = l.ID
				all = append(all, paths.PathMatch{Path: m})
			}
		}
	}
	out := map[string][]string{}
	for _, m := range paths.DedupContained(all) {
		id := owner[m.Path]
		out[id] = append(out[id], m.Path)
	}
	return out
}

// HasAppleComponent는 경로에 com.apple.* 디렉토리가 끼어 있는지 본다(순수 함수 — 테스트 대상).
// 와일드카드 위치(~/Library/Application Support/*/Cache)가 애플 기본 앱 데이터를 삼키는 걸 막는다.
// paths.IsProtected는 경로 접두사 블랙리스트라 ~/Library 아래의 애플 데이터는 못 걸러낸다.
func HasAppleComponent(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if strings.HasPrefix(part, "com.apple.") {
			return true
		}
	}
	return false
}

// Usage는 위치 안에서 무엇이 얼마를 쓰는지 한 줄이다(앱별 내역).
type Usage struct {
	Name   string `json:"name"`
	SizeKB int    `json:"size_kb"`
}

// appSupportDirName은 ~/Library/Application Support/<앱>/... 경로에서 <앱>을 뽑는다(순수 함수).
// 그 아래가 아니면 ""(그룹 없음).
func appSupportDirName(home, path string) string {
	base := filepath.Join(home, "Library", "Application Support") + string(filepath.Separator)
	rest, ok := strings.CutPrefix(path, base)
	if !ok || rest == "" {
		return ""
	}
	if i := strings.Index(rest, string(filepath.Separator)); i > 0 {
		return rest[:i]
	}
	return rest
}

// GroupUsage는 경로별 용량을 groupBy 기준으로 묶어 용량순으로 돌려준다(순수 함수 — 테스트 대상).
// groupBy가 비었거나 알 수 없으면 nil(내역 없음).
func GroupUsage(home, groupBy string, sizes map[string]int) []Usage {
	if groupBy != GroupByAppSupportDir {
		return nil
	}
	byName := map[string]int{}
	for path, kb := range sizes {
		if name := appSupportDirName(home, path); name != "" {
			byName[name] += kb
		}
	}
	out := make([]Usage, 0, len(byName))
	for name, kb := range byName {
		out = append(out, Usage{Name: name, SizeKB: kb})
	}
	sortUsage(out)
	return out
}

// sortUsage는 용량 내림차순, 같으면 이름순으로 정렬한다(출력이 실행마다 흔들리지 않게).
func sortUsage(u []Usage) {
	sort.Slice(u, func(i, j int) bool {
		if u[i].SizeKB != u[j].SizeKB {
			return u[i].SizeKB > u[j].SizeKB
		}
		return u[i].Name < u[j].Name
	})
}

// UserHome은 홈 디렉토리다(조회 실패 시 빈 문자열).
func UserHome() string {
	home, _ := os.UserHomeDir()
	return home
}

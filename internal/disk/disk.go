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
	Tier string `json:"tier"`
	// Cleanup은 정리에 쓸 네이티브 명령이다(rm 아님). 비어 있으면 리포트 전용.
	Cleanup [][]string `json:"cleanup,omitempty"`
	// Trash는 네이티브 명령이 없어 휴지통 이동으로 정리하는 위치다(DerivedData 등).
	Trash bool   `json:"trash,omitempty"`
	Note  string `json:"note,omitempty"`
}

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
				if paths.IsProtected(m) {
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

// UserHome은 홈 디렉토리다(조회 실패 시 빈 문자열).
func UserHome() string {
	home, _ := os.UserHomeDir()
	return home
}

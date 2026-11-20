package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

// EOL(지원 종료) 정보는 endoflife.date에서 가져온다.
//
// 목적은 **최신 강요가 아니라 판단 근거 제공**이다. 몇 달 만에 어떤 스택으로 돌아왔을 때
// "깔려 있나 · 몇 버전인가 · 올려야 하나"에 한 번에 답하려는 것이다.
//
// 원칙:
//   - 네트워크 실패·오프라인은 **에러가 아니다**. 조용히 생략하고 설치 현황만 보여준다.
//   - 런타임 메이저 업그레이드는 프로젝트를 깨뜨린다. "업데이트 대기"로 뭉뚱그리지 않고
//     자동 실행 대상에도 넣지 않는다 — 쓸 명령을 **보여주기만** 한다.

// 상태 상수와 배지 렌더는 report에 있다(report.Runtime의 필드를 해석하는 일이라).
// 여기서는 별칭으로 노출해 source 안의 판정 코드가 짧게 쓰게 한다.
const (
	EOLUnknown   = report.EOLUnknown
	EOLSupported = report.EOLSupported
	EOLSoon      = report.EOLSoon
	EOLPassed    = report.EOLPassed
	EOLRolling   = report.EOLRolling

	UpdateUnknown = report.UpdateUnknown
	UpdateCurrent = report.UpdateCurrent
	UpdatePatch   = report.UpdatePatch
	UpdateMajor   = report.UpdateMajor
)

// EOLBadge/UpdateBadge는 report의 것을 그대로 노출한다(CLI/UI 공용).
var (
	EOLBadge    = report.EOLBadge
	UpdateBadge = report.UpdateBadge
)

// eolProducts는 런타임 이름 → endoflife.date 제품 슬러그다.
// java는 `java`/`openjdk` 슬러그가 없어(404) OpenJDK 수명주기의 사실상 기준인
// eclipse-temurin(Adoptium)을 쓴다. dotnet/swift는 대상에서 뺐다(요구 목록에 없음).
var eolProducts = map[string]string{
	"python": "python",
	"node":   "nodejs",
	"go":     "go",
	"java":   "eclipse-temurin",
	"ruby":   "ruby",
	"php":    "php",
	"rust":   "rust",
}

// eolCycle은 endoflife.date의 사이클 한 개다.
// eol/support 필드는 날짜 문자열이거나 false(아직 미정)라 any로 받는다.
type eolCycle struct {
	Cycle  string `json:"cycle"`
	Latest string `json:"latest"`
	EOL    any    `json:"eol"`
	LTS    any    `json:"lts"`
}

// eolCacheEntry는 제품 하나의 캐시다.
type eolCacheEntry struct {
	FetchedAt time.Time  `json:"fetched_at"`
	Cycles    []eolCycle `json:"cycles"`
}

type eolCache map[string]eolCacheEntry

// eolCacheTTL은 캐시 유효기간이다. 릴리스 주기가 길어 7일이면 충분하고,
// 매 실행마다 7번씩 외부 요청을 보내지 않게 한다.
const eolCacheTTL = 7 * 24 * time.Hour

// eolHTTPTimeout은 요청 하나의 상한이다. 조회 실패는 조용히 생략하므로 짧게 잡는다.
const eolHTTPTimeout = 6 * time.Second

// EOLCachePath는 캐시 파일 경로다(~/.macinv/eol-cache.json).
func EOLCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".macinv", "eol-cache.json")
}

// EnrichEOL은 런타임 목록에 EOL/최신버전/업데이트 상태/권장 명령을 채운다.
// offline이면 네트워크 조회 없이 캐시만 쓴다(캐시도 없으면 조용히 생략).
// **어떤 실패도 에러로 올리지 않는다** — 이 정보는 부가 정보고, 설치 현황은 그대로 보여야 한다.
func EnrichEOL(ctx context.Context, runtimes []report.Runtime, managers []report.VersionManager, offline bool, now time.Time) {
	cache := loadEOLCache()
	changed := false
	hasMise := hasManager(managers, MgrMise)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range runtimes {
		rt := &runtimes[i]
		product, ok := eolProducts[rt.Name]
		if !ok || !rt.Installed {
			continue
		}
		// 아래 셋은 네트워크 없이도 낼 수 있다(관리 주체만 알면 된다).
		rt.InstalledVersion = rt.Version
		rt.Manager = managerOf(*rt)
		rt.SuggestedCommand = "" // 최신 사이클을 알고 나서 applyEOL이 채운다

		wg.Add(1)
		go func(rt *report.Runtime, product string) {
			defer wg.Done()
			mu.Lock()
			entry, fresh := cache[product], false
			if !entry.FetchedAt.IsZero() && now.Sub(entry.FetchedAt) < eolCacheTTL {
				fresh = true
			}
			mu.Unlock()

			cycles := entry.Cycles
			if !fresh && !offline {
				if fetched, err := fetchEOL(ctx, product); err == nil && len(fetched) > 0 {
					cycles = fetched
					mu.Lock()
					cache[product] = eolCacheEntry{FetchedAt: now, Cycles: fetched}
					changed = true
					mu.Unlock()
				}
			}
			// 오프라인 + 캐시 없음이면 EOL 정보만 비워 둔다(조용히 생략).
			if len(cycles) > 0 {
				applyEOL(rt, cycles, now)
			}
			// 권장 명령은 네트워크 없이도 낼 수 있다 — 관리 주체만 알면 된다.
			// (최신 사이클을 모르면 명령 안의 버전이 latest로 나간다.)
			rt.SuggestedCommand = SuggestUpgradeCommand(SuggestCtx{
				Runtime: rt.Name, Manager: rt.Manager, LatestCycle: rt.LatestCycle, HasMise: hasMise,
			})
		}(rt, product)
	}
	wg.Wait()

	if changed {
		saveEOLCache(cache)
	}
}

// applyEOL은 사이클 목록에서 이 런타임에 해당하는 정보를 골라 채운다(순수 함수 — 테스트 대상).
func applyEOL(rt *report.Runtime, cycles []eolCycle, now time.Time) {
	cyc, ok := MatchCycle(rt.Version, cycles)
	if !ok {
		return
	}
	rt.EOLCycle = cyc.Cycle
	rt.LatestVersion = normalizeLatest(cyc.Latest)
	rt.LatestCycle = newestCycle(cycles)
	rt.EOLDate, rt.EOLStatus = eolDateStatus(cyc.EOL, now)
	rt.UpdateStatus = compareVersions(rt.Version, rt.LatestVersion, rt.LatestCycle, cyc.Cycle)
}

// MatchCycle은 설치 버전이 속한 사이클을 찾는다(순수 함수 — 테이블 테스트 대상).
// 사이클은 "3.9"(python), "26"(node/java), "1.26"(go/rust)처럼 자릿수가 제각각이라
// **가장 긴 접두사 일치**로 고른다 — "1.2"가 "1.26.5"를 잘못 집지 않게.
func MatchCycle(version string, cycles []eolCycle) (eolCycle, bool) {
	v := normalizeVersion(version)
	if v == "" {
		return eolCycle{}, false
	}
	var best eolCycle
	found := false
	for _, c := range cycles {
		if c.Cycle == "" {
			continue
		}
		if v == c.Cycle || strings.HasPrefix(v, c.Cycle+".") {
			if !found || len(c.Cycle) > len(best.Cycle) {
				best, found = c, true
			}
		}
	}
	return best, found
}

// normalizeVersion은 버전 문자열에서 숫자 버전만 남긴다(순수 함수 — 테스트 대상).
// "openjdk 17.0.9" → "17.0.9", "v25.9.0" → "25.9.0", "go1.26.5" → "1.26.5".
func normalizeVersion(s string) string {
	if m := versionRe.FindString(s); m != "" {
		return m
	}
	// "17" 처럼 점이 없는 메이저 단독도 받아들인다.
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		} else if digits.Len() > 0 {
			break
		}
	}
	return digits.String()
}

// normalizeLatest는 endoflife.date의 latest에서 빌드 메타데이터를 뗀다("26.0.2+10" → "26.0.2").
func normalizeLatest(s string) string {
	if i := strings.IndexAny(s, "+-"); i > 0 {
		return s[:i]
	}
	return s
}

// eolDateStatus는 eol 필드(날짜 문자열 또는 false)를 날짜와 상태로 바꾼다(순수 함수 — 테스트 대상).
func eolDateStatus(raw any, now time.Time) (date, status string) {
	switch v := raw.(type) {
	case string:
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return "", EOLUnknown
		}
		switch {
		case t.Before(now):
			return v, EOLPassed
		case t.Before(now.AddDate(1, 0, 0)):
			return v, EOLSoon
		default:
			return v, EOLSupported
		}
	case bool:
		// false = 종료일 미정(아직 지원). go/rust처럼 굴러가는 릴리스에서 흔하다.
		if !v {
			return "", EOLRolling
		}
		return "", EOLPassed
	}
	return "", EOLUnknown
}

// newestCycle은 목록에서 가장 최신 사이클 문자열을 돌려준다(endoflife.date는 최신순 정렬).
func newestCycle(cycles []eolCycle) string {
	if len(cycles) == 0 {
		return ""
	}
	return cycles[0].Cycle
}

// compareVersions는 업데이트 상태를 세 갈래로 나눈다(순수 함수 — 테이블 테스트 대상).
// 메이저와 패치를 구분하는 게 핵심이다: 런타임 메이저 업그레이드는 프로젝트를 깨뜨리므로
// "업데이트 대기"로 뭉뚱그리면 안 된다.
func compareVersions(installed, latestInCycle, newest, myCycle string) string {
	if installed == "" {
		return UpdateUnknown
	}
	if newest != "" && myCycle != "" && newest != myCycle {
		return UpdateMajor
	}
	if latestInCycle == "" {
		return UpdateUnknown
	}
	if compareSemver(normalizeVersion(installed), normalizeVersion(latestInCycle)) < 0 {
		return UpdatePatch
	}
	return UpdateCurrent
}

// compareSemver는 점으로 나뉜 숫자 버전을 비교한다(순수 함수). a<b면 음수.
func compareSemver(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

// hasManager는 감지된 버전 매니저 중에 name이 있는지 본다.
func hasManager(managers []report.VersionManager, name string) bool {
	for _, m := range managers {
		if m.Name == name {
			return true
		}
	}
	return false
}

// managerOf는 활성 경로의 관리 주체를 돌려준다.
func managerOf(rt report.Runtime) string {
	if len(rt.Origins) > 0 {
		return rt.Origins[0].Manager
	}
	return ""
}

// SuggestCtx는 권장 명령을 만드는 데 필요한 맥락이다(순수 함수 입력).
type SuggestCtx struct {
	Runtime     string // node|python|go|...
	Manager     string // 활성 경로의 관리 주체
	LatestCycle string // 나와 있는 가장 최신 사이클(3.14, 26 …)
	HasMise     bool   // mise가 설치돼 있는지(시스템 런타임 대안 제시용)
}

// SuggestUpgradeCommand는 이 런타임을 올릴 때 쓸 명령을 만든다(순수 함수 — 테이블 테스트 대상).
// **실행하지 않는다** — 런타임 업그레이드는 프로젝트를 깨뜨릴 수 있어 사용자가 직접 판단해야 한다.
// 관리 주체를 모르면 빈 문자열: 엉뚱한 명령을 권하느니 아무 말도 안 하는 게 낫다.
func SuggestUpgradeCommand(c SuggestCtx) string {
	tool := toolNameOf(c.Runtime)
	cycle := c.LatestCycle
	if cycle == "" {
		cycle = "latest"
	}
	switch c.Manager {
	case MgrBrew:
		return "brew upgrade " + brewFormulaOf(c.Runtime)
	case MgrNvm:
		return "nvm install --lts && nvm alias default lts/*"
	case MgrPyenv:
		return "pyenv install " + cycle + " && pyenv global " + cycle
	case MgrRustup:
		return "rustup update"
	case MgrMise:
		return "mise use -g " + tool + "@" + cycle
	case MgrAsdf:
		return "asdf install " + tool + " latest && asdf global " + tool + " latest"
	case MgrSdkman:
		return "sdk upgrade " + c.Runtime
	case MgrSystem:
		// 시스템 제공 런타임은 **제자리 업그레이드가 안 된다**(OS가 소유한다).
		// 따로 설치해 PATH 앞에 두는 게 유일한 길이라 그 명령을 권한다.
		if c.HasMise {
			return "mise use -g " + tool + "@" + cycle + "   (시스템 버전은 그대로 두고 앞에 둡니다)"
		}
		return "brew install " + brewFormulaOf(c.Runtime) + "   (시스템 버전은 업그레이드할 수 없습니다)"
	}
	return ""
}

// brewFormulaOf는 런타임 이름 → brew 포뮬러 이름이다(다른 것만 매핑).
func brewFormulaOf(runtime string) string {
	if runtime == "java" {
		return "openjdk"
	}
	return runtime
}

// toolNameOf는 런타임 이름 → mise/asdf 플러그인 이름이다.
func toolNameOf(runtime string) string { return runtime }

// ---- 조회 / 캐시 -----------------------------------------------------------

// fetchEOL은 endoflife.date에서 제품의 사이클 목록을 받아온다.
func fetchEOL(ctx context.Context, product string) ([]eolCycle, error) {
	cctx, cancel := context.WithTimeout(ctx, eolHTTPTimeout)
	defer cancel()

	url := "https://endoflife.date/api/" + product + ".json"
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	// 404면 HTML이 온다 — 크기를 제한해 이상한 응답에 메모리를 쓰지 않는다.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var cycles []eolCycle
	if err := json.Unmarshal(body, &cycles); err != nil {
		return nil, err
	}
	return cycles, nil
}

// loadEOLCache는 캐시를 읽는다. 없거나 깨졌으면 빈 캐시(실패는 조용히 무시).
func loadEOLCache() eolCache {
	data, err := os.ReadFile(EOLCachePath())
	if err != nil {
		return eolCache{}
	}
	var c eolCache
	if json.Unmarshal(data, &c) != nil {
		return eolCache{}
	}
	return c
}

// saveEOLCache는 캐시를 기록한다(실패는 조용히 무시 — 부가 정보다).
func saveEOLCache(c eolCache) {
	path := EOLCachePath()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	if data, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

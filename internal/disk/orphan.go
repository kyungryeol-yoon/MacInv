package disk

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// 고아 캐시 = 캐시 디렉토리는 남아 있는데 그걸 만든 도구가 인벤토리에 없는 상태.
// (예: gradle을 지웠는데 ~/.gradle에 3GB가 남아 있다.)
//
// 이건 **인벤토리를 가진 macinv만 할 수 있는 판단**이다. du/ncdu는 폴더가 크다는 것만 알려주고
// "이건 이제 아무도 안 쓴다"는 말은 못 한다. 그래서 요약에서 눈에 띄게 표시한다.

// IsOrphanCache는 이 캐시를 쓰는 것이 하나도 안 남았는지 본다(순수 함수 — 테이블 테스트 대상).
//
// 보수적 원칙 — 애매하면 고아가 아니라고 본다:
//   - 소유 도구를 모르면(Tool이 비었으면) 판정하지 않는다.
//   - never 등급(사용자 데이터)은 고아 표시를 하지 않는다 — 지우라는 신호로 읽히면 안 된다.
//   - **CLI 도구가 없어도 GUI 앱이 그 캐시를 쓴다**: gradle CLI 없이 Android Studio만
//     깔아도 ~/.gradle은 살아있는 캐시다. OwnerApps 중 하나라도 설치돼 있으면 고아가 아니다.
//
// installed는 DetectTools가 만든 집합이다(인벤토리 ∪ PATH). 앱 이름도 들어 있다.
func IsOrphanCache(l Location, installed map[string]bool) bool {
	if l.Tool == "" || l.Tier == TierNever {
		return false
	}
	if anyInstalled(installed, l.Tool) {
		return false
	}
	for _, app := range l.OwnerApps {
		if anyInstalled(installed, app) {
			return false
		}
	}
	return true
}

// anyInstalled는 이름의 정규화 형태 중 하나라도 설치 집합에 있는지 본다.
func anyInstalled(installed map[string]bool, name string) bool {
	for _, k := range toolKeys(name) {
		if installed[k] {
			return true
		}
	}
	return false
}

// toolKeys는 이름 하나를 설치 집합의 키 후보들로 바꾼다(순수 함수).
// 공백 유무가 갈리는 이름이 있어("Android Studio" ↔ "androidstudio") 둘 다 만든다.
// InstalledTools가 **넣을 때**와 IsOrphanCache가 **찾을 때** 같은 함수를 써야
// 한쪽만 바뀌어 매칭이 깨지는 일이 없다.
func toolKeys(name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return nil
	}
	packed := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(lower)
	if packed == lower {
		return []string{lower}
	}
	return []string{lower, packed}
}

// InstalledTools는 인벤토리에서 "설치된 도구 이름" 집합을 만든다(순수 함수 — 테스트 대상).
// 이름 하나를 여러 소스가 줄 수 있어(brew formula git / 런타임 go / ~/go/bin) 전부 합친다.
// 앱은 Bundle ID의 마지막 마디까지 넣는다 — OrbStack.app이 orbstack 캐시의 주인임을 알아보려면 필요하다.
func InstalledTools(rep *report.Report) map[string]bool {
	set := map[string]bool{}
	add := func(s string) {
		for _, k := range toolKeys(s) {
			set[k] = true
		}
	}
	if rep == nil {
		return set
	}
	s := rep.Sections
	if s.Brew != nil {
		add("brew") // 섹션이 있다는 것 자체가 brew 설치의 증거다(포뮬러 목록에는 brew가 없다)
		for _, f := range s.Brew.Formulae {
			add(f.Name)
		}
		for _, c := range s.Brew.Casks {
			add(c.Name)
		}
	}
	if s.Npm != nil {
		add("npm")
		add("node")
		for _, p := range s.Npm.Packages {
			add(p.Name)
		}
	}
	if len(s.Cargo) > 0 {
		add("cargo")
	}
	if len(s.GoBin) > 0 {
		add("go")
	}
	for _, p := range s.Pipx {
		add(p.Name)
	}
	for _, p := range s.Cargo {
		add(p.Name)
	}
	for _, g := range s.GoBin {
		add(g.Name)
	}
	for _, a := range s.Apps {
		add(a.Name) // toolKeys가 "android studio"/"androidstudio" 둘 다 넣는다
		if a.BundleID != nil {
			if i := strings.LastIndex(*a.BundleID, "."); i >= 0 {
				add((*a.BundleID)[i+1:])
			}
		}
	}
	for _, rt := range rep.Runtimes {
		if rt.Installed {
			add(rt.Name)
		}
	}
	// 런타임과 함께 오는 도구는 이름이 다르다 — 별칭을 채운다.
	for tool, owner := range toolAliases {
		if set[owner] {
			set[tool] = true
		}
	}
	return set
}

// toolAliases는 "이 도구가 있으면 저 도구도 있다"는 매핑이다(도구 → 그것을 데려오는 것).
// npm은 node와, pip는 python과 함께 온다. 별칭이 없으면 멀쩡한 캐시가 고아로 오탐된다.
var toolAliases = map[string]string{
	"npm":        "node",
	"npx":        "node",
	"pip":        "python",
	"pip3":       "python",
	"xcodebuild": "xcode",
	"simctl":     "xcode",
	"mvn":        "java",
}

// toolBinaries는 도구 이름과 실제 실행 파일 이름이 다를 때의 후보다(PATH 확인용).
// pip는 macOS에 pip3로만 있고, simctl은 xcrun의 서브커맨드다.
var toolBinaries = map[string][]string{
	"pip":             {"pip", "pip3"},
	"python":          {"python3", "python"},
	"simctl":          {"xcrun"},
	"orbstack":        {"orb", "orbctl"},
	"huggingface-cli": {"huggingface-cli", "hf"},
}

// DetectTools는 인벤토리와 PATH를 합쳐 "설치된 도구" 집합을 만든다.
//
// 인벤토리만 쓰면 오탐이 난다: 캐시가 오래돼 runtimes가 비어 있으면 python이 없다고 보고
// pip 캐시를 고아로 잘못 표시한다(실측). PATH는 오래되지 않는 ground truth라 합집합을 쓴다.
// 반대로 PATH만 쓰면 PATH에 없는 앱(OrbStack 등)을 놓치므로 둘 다 필요하다.
func DetectTools(rep *report.Report) map[string]bool {
	return detectToolsFrom(rep, Locations())
}

// detectToolsFrom은 위치 목록을 주입받는 형태다(테스트용 — 실제 PATH 조회는 그대로 한다).
func detectToolsFrom(rep *report.Report, locs []Location) map[string]bool {
	set := InstalledTools(rep)
	for _, l := range locs {
		if l.Tool == "" || anyInstalled(set, l.Tool) {
			continue
		}
		cands := toolBinaries[l.Tool]
		if len(cands) == 0 {
			cands = []string{l.Tool}
		}
		for _, bin := range cands {
			if _, err := exec.LookPath(bin); err == nil {
				for _, k := range toolKeys(l.Tool) {
					set[k] = true
				}
				break
			}
		}
	}
	return set
}

// parseDuLines는 `du -sk` 출력을 경로→KB로 파싱한다(순수 함수 — 테스트 대상).
// 경로에 공백이 있을 수 있어 첫 공백 구간까지만 숫자로 보고 나머지를 통째로 경로로 쓴다.
func parseDuLines(out string) map[string]int {
	sizes := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		i := strings.IndexAny(line, " \t")
		if i <= 0 {
			continue
		}
		n, err := strconv.Atoi(line[:i])
		if err != nil {
			continue
		}
		if p := strings.TrimLeft(line[i:], " \t"); p != "" {
			sizes[p] = n
		}
	}
	return sizes
}

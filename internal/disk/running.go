package disk

import (
	"context"
	"path/filepath"
	"strings"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
)

// 실행 중인 Electron 앱 보호.
//
// Chromium은 캐시 파일 핸들을 **연 채로** 돈다. 앱이 켜져 있을 때 캐시 디렉토리를
// 휴지통으로 옮기면 앱이 오작동하거나 캐시를 이상하게 재생성한다.
// Notion·Slack을 켜둔 채 `disk --apply`를 돌리는 게 가장 흔한 상황이라 실사용에서 바로 부딪힌다.
//
// 그래서 계획 단계에서 소유 앱이 실행 중인 경로를 **기본적으로 제외**하고,
// `--include-running`을 줬을 때만 경고와 함께 포함한다.

// RunningApps는 실행 중인 프로세스에서 앱 번들 이름을 모은다(읽기 전용).
//
// ok=false는 **조회 자체가 실패했다**는 뜻이다. "실행 중인 앱이 없다"(빈 목록 + ok=true)와
// 반드시 구분해야 한다 — 둘을 뭉뚱그리면 보호가 조용히 꺼진 채 사용자는 도는 줄 안다.
// **안전장치가 조용히 사라지는 건 처음부터 없는 것보다 나쁘다.**
func RunningApps(ctx context.Context, run *iexec.Runner) (apps []string, ok bool) {
	// comm=은 실행 파일 **전체 경로**를 준다(/Applications/Visual Studio Code.app/Contents/MacOS/Electron).
	// 프로세스 이름(Electron, Code Helper …)보다 앱 번들 이름이 훨씬 믿을 만하다.
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 15}, "ps", "-Axo", "comm=")
	if err != nil || strings.TrimSpace(out) == "" {
		// 출력이 통째로 비는 것도 실패다 — macOS에서 프로세스가 0개일 수는 없다.
		return nil, false
	}
	return ParseRunningApps(out), true
}

// ParseRunningApps는 ps 출력에서 .app 번들 이름을 뽑는다(순수 함수 — 테스트 대상).
// 한 경로에 .app이 여러 개면(헬퍼가 앱 안에 중첩) **전부** 담는다 —
// 놓쳐서 실행 중인 앱의 캐시를 지우는 것보다 넉넉히 잡아 제외하는 쪽이 안전하다.
func ParseRunningApps(psOut string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(psOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, part := range strings.Split(line, string(filepath.Separator)) {
			name, ok := strings.CutSuffix(part, ".app")
			if !ok || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// dirToAppAliases는 데이터 디렉토리 이름과 앱 이름이 다른 알려진 경우다.
// 대부분은 단어 매칭으로 잡히지만("Code" ⊂ "Visual Studio Code"), 규칙으로 안 되는 건 여기 적는다.
var dirToAppAliases = map[string][]string{
	"code":            {"Visual Studio Code"},
	"code - insiders": {"Visual Studio Code - Insiders"},
	"chatgpt":         {"ChatGPT"},
}

// MatchesRunningApp은 데이터 디렉토리 이름이 실행 중인 앱에 해당하는지 본다
// (순수 함수 — 테이블 테스트 대상).
//
// 디렉토리 이름과 앱 이름은 자주 다르다("Code" ↔ "Visual Studio Code",
// "LogiOptionsPlus" ↔ "logioptionsplus_agent"). 세 규칙을 순서대로 본다:
//  1. 정규화 후 완전 일치
//  2. 별칭 표
//  3. 앱 이름을 단어로 쪼갰을 때 디렉토리 이름이 **한 단어로** 들어 있음
//
// 3번에서 부분 문자열이 아니라 단어 단위를 쓰는 이유: "Home"/"Mail" 같은 짧은 이름이
// 무관한 앱에 걸리면 멀쩡한 캐시가 영영 정리되지 않는다.
// 다만 판정이 애매할 때는 **제외(=보호) 쪽으로 기운다** — 잘못 지우는 쪽이 훨씬 나쁘다.
func MatchesRunningApp(dirName string, runningApps []string) bool {
	dir := normalizeAppKey(dirName)
	if dir == "" {
		return false
	}
	aliases := dirToAppAliases[strings.ToLower(strings.TrimSpace(dirName))]
	for _, app := range runningApps {
		if normalizeAppKey(app) == dir {
			return true
		}
		for _, a := range aliases {
			if normalizeAppKey(a) == normalizeAppKey(app) {
				return true
			}
		}
		if len(dir) >= 3 && hasWord(app, dir) {
			return true
		}
	}
	return false
}

// hasWord는 앱 이름을 단어로 쪼갰을 때 정규화된 word가 한 단어로 들어 있는지 본다.
// 구분자는 공백과 흔한 이름 구분 기호(_ - .)다.
func hasWord(appName, word string) bool {
	for _, part := range strings.FieldsFunc(appName, func(r rune) bool {
		return r == ' ' || r == '_' || r == '-' || r == '.'
	}) {
		if normalizeAppKey(part) == word {
			return true
		}
	}
	return false
}

// normalizeAppKey는 비교용으로 공백/구분자를 지우고 소문자화한다.
func normalizeAppKey(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".app")
	s = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s)
	return strings.ToLower(s)
}

// blockedByRunning은 위치의 경로들 중 소유 앱이 실행 중인 것을 찾는다(경로 → 앱 이름).
// GroupBy가 app-support-dir인 위치(= Electron 캐시)에만 의미가 있다.
func blockedByRunning(home, groupBy string, paths []string, running []string) map[string]string {
	if groupBy != GroupByAppSupportDir || len(running) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, p := range paths {
		app := appSupportDirName(home, p)
		if app != "" && MatchesRunningApp(app, running) {
			out[p] = app
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RunningBlockedUsage는 실행 중이라 제외된 앱별 용량을 용량순으로 모은다(순수 함수 — 테스트 대상).
// "3개 앱이 실행 중이라 제외됨 (총 12GB)"를 내려면 앱 단위 합계가 필요하다.
func RunningBlockedUsage(entries []Entry) []Usage {
	byApp := map[string]int{}
	for _, e := range entries {
		for path, app := range e.BlockedByRunning {
			byApp[app] += e.PathSizes[path]
		}
	}
	if len(byApp) == 0 {
		return nil
	}
	out := make([]Usage, 0, len(byApp))
	for name, kb := range byApp {
		out = append(out, Usage{Name: name, SizeKB: kb})
	}
	sortUsage(out)
	return out
}

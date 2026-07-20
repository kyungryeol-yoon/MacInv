package source

import (
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// Runtime 타입은 report 패키지로 이동했다. 별칭으로 노출한다.
type Runtime = report.Runtime

// runtimeSpec은 런타임 하나의 조회 방법이다.
type runtimeSpec struct {
	name  string
	bin   string
	args  []string
	verRe *regexp.Regexp // nil이면 일반 추출. 지정 시 capture group 1을 버전으로 사용.
}

// swift는 "swift-driver version"이 먼저 나와 일반 추출이 드라이버 버전을 잡으므로 전용 정규식이 필요하다.
var swiftRe = regexp.MustCompile(`Apple Swift version (\d+\.\d+(?:\.\d+)?)`)

// 대상 런타임. 활성 버전은 RunCombined로 stdout+stderr를 합쳐 파싱한다(java/swift는 stderr로 출력).
var runtimeSpecs = []runtimeSpec{
	{name: "node", bin: "node", args: []string{"--version"}},
	{name: "python", bin: "python3", args: []string{"--version"}},
	{name: "go", bin: "go", args: []string{"version"}},
	{name: "rust", bin: "rustc", args: []string{"--version"}},
	{name: "java", bin: "java", args: []string{"-version"}},
	{name: "ruby", bin: "ruby", args: []string{"--version"}},
	{name: "php", bin: "php", args: []string{"--version"}},
	{name: "dotnet", bin: "dotnet", args: []string{"--version"}},
	{name: "swift", bin: "swift", args: []string{"--version"}, verRe: swiftRe},
}

// ScanRuntimes는 모든 런타임을 병렬로 조회한다. 미설치는 Installed=false로 표기.
func ScanRuntimes(ctx context.Context, run *iexec.Runner, env TraceEnv) []Runtime {
	out := make([]Runtime, len(runtimeSpecs))
	var wg sync.WaitGroup
	for i, spec := range runtimeSpecs {
		wg.Add(1)
		go func(i int, spec runtimeSpec) {
			defer wg.Done()
			out[i] = scanRuntime(ctx, run, spec, env)
		}(i, spec)
	}
	wg.Wait()
	return out
}

func scanRuntime(ctx context.Context, run *iexec.Runner, spec runtimeSpec, env TraceEnv) Runtime {
	rt := Runtime{Name: spec.name}
	origins := TraceManaged(ctx, run, spec.bin, env)
	if len(origins) == 0 {
		return rt // 미설치
	}
	rt.Installed = true
	rt.Origins = origins
	rt.Conflict = CountResolved(origins) > 1

	// 활성(첫) 경로로 버전 조회.
	if ver, err := run.RunCombined(ctx, iexec.Opt{TimeoutSec: 15}, spec.bin, spec.args...); err == nil {
		rt.Version = extractVersionRe(ver, spec.verRe)
	}
	return rt
}

// CountResolved는 origins의 서로 다른 실제 경로 개수를 센다(PATH 충돌 판정용).
func CountResolved(origins []Origin) int {
	set := map[string]bool{}
	for _, o := range origins {
		set[o.Resolved] = true
	}
	return len(set)
}

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// extractVersion은 버전 명령 출력에서 첫 버전 토큰(예: 22.22.2, 6.0)을 뽑는다(순수 함수 — 테스트 대상).
func extractVersion(out string) string {
	return extractVersionRe(out, nil)
}

// extractVersionRe는 re가 있으면 capture group 1을, 없으면 일반 버전 토큰을 뽑는다.
func extractVersionRe(out string, re *regexp.Regexp) string {
	if re != nil {
		if m := re.FindStringSubmatch(out); m != nil {
			return m[1]
		}
		// 전용 정규식이 실패하면 일반 추출로 폴백.
	}
	return versionRe.FindString(out)
}

// ---- 버전 매니저 ----------------------------------------------------------

// VersionManager 타입은 report 패키지로 이동했다. 별칭으로 노출한다.
type VersionManager = report.VersionManager

// vmSpec은 버전 매니저 하나를 홈 아래 디렉토리 구조로 탐지하는 방법이다.
type vmSpec struct {
	name     string
	glob     string // 홈 기준 상대 glob. twoLevel이면 <tool>/<version> 구조.
	twoLevel bool
}

var vmSpecs = []vmSpec{
	{MgrNvm, ".nvm/versions/node/*", false},
	{MgrPyenv, ".pyenv/versions/*", false},
	{MgrRustup, ".rustup/toolchains/*", false},
	{MgrAsdf, ".asdf/installs/*/*", true},
	{MgrMise, ".local/share/mise/installs/*/*", true},
	{MgrSdkman, ".sdkman/candidates/*/*", true},
}

// ScanVersionManagers는 감지된 버전 매니저와 관리 버전 목록을 돌려준다(파일시스템만 사용, 외부 명령 없음).
func ScanVersionManagers(env TraceEnv) []VersionManager {
	var out []VersionManager
	for _, vm := range vmSpecs {
		matches, err := filepath.Glob(filepath.Join(env.Home, vm.glob))
		if err != nil || len(matches) == 0 {
			continue
		}
		versions := versionsFromMatches(matches, vm.twoLevel)
		if len(versions) == 0 {
			continue
		}
		out = append(out, VersionManager{Name: vm.name, Versions: versions})
	}
	return out
}

// versionsFromMatches는 glob 결과를 버전 문자열로 바꾼다(순수 함수 — 테스트 대상).
// twoLevel이면 마지막 두 경로 조각을 "<tool> <version>"으로, 아니면 마지막 조각을 버전으로 쓴다.
func versionsFromMatches(matches []string, twoLevel bool) []string {
	var vs []string
	for _, m := range matches {
		base := filepath.Base(m)
		if base == "current" || strings.HasPrefix(base, ".") {
			continue // sdkman의 current 심볼릭 등 제외
		}
		if twoLevel {
			tool := filepath.Base(filepath.Dir(m))
			vs = append(vs, tool+" "+base)
		} else {
			vs = append(vs, base)
		}
	}
	sort.Strings(vs)
	return vs
}

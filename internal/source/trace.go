package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// 관리 주체(Manager) 값. M4 runtimes에서도 재사용한다.
const (
	MgrBrew   = "brew"
	MgrNpm    = "npm"
	MgrCargo  = "cargo"
	MgrPipx   = "pipx"
	MgrGo     = "go"
	MgrSystem = "system"
	MgrManual = "manual"

	// 버전 매니저(runtimes 전용).
	MgrNvm    = "nvm"
	MgrPyenv  = "pyenv"
	MgrMise   = "mise"
	MgrAsdf   = "asdf"
	MgrRustup = "rustup"
	MgrSdkman = "sdkman"
)

// Origin 타입은 report 패키지로 이동했다(scan/tui가 import 순환 없이 쓰도록). 별칭으로 노출한다.
type Origin = report.Origin

// TraceEnv는 분류에 필요한 환경 정보다(테스트에서 주입 가능하도록 분리).
type TraceEnv struct {
	GoBin string // $GOBIN 또는 ~/go/bin
	Home  string
}

// CurrentTraceEnv는 현재 프로세스 환경으로 TraceEnv를 만든다.
func CurrentTraceEnv() TraceEnv {
	home, _ := os.UserHomeDir()
	return TraceEnv{GoBin: goBinDir(), Home: home}
}

// classifier는 해소된 경로를 관리 주체/패키지로 분류하는 함수다.
type classifier func(resolved string, env TraceEnv) (manager, pkg string)

// Trace는 which -a로 name의 모든 경로를 찾아 각각의 소유권(패키지 관점)을 역추적한다.
func Trace(ctx context.Context, run *iexec.Runner, name string, env TraceEnv) []Origin {
	return traceWith(ctx, run, name, env, Classify)
}

// TraceManaged는 Trace와 같지만 버전 매니저(nvm/pyenv/...)를 우선 인식한다(runtimes용).
func TraceManaged(ctx context.Context, run *iexec.Runner, name string, env TraceEnv) []Origin {
	return traceWith(ctx, run, name, env, ClassifyManaged)
}

func traceWith(ctx context.Context, run *iexec.Runner, name string, env TraceEnv, fn classifier) []Origin {
	// which는 못 찾으면 exit 1을 내므로 허용.
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 10, OKCodes: []int{0, 1}}, "which", "-a", name)
	if err != nil {
		return nil
	}
	var origins []Origin
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		p := strings.TrimSpace(line)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		resolved := p
		if r, err := filepath.EvalSymlinks(p); err == nil {
			resolved = r
		}
		mgr, pkg := fn(resolved, env)
		origins = append(origins, Origin{Path: p, Resolved: resolved, Manager: mgr, Package: pkg})
	}
	if len(origins) > 0 {
		origins[0].Active = true // which -a는 PATH 순서 → 첫 번째가 실제 실행됨
	}
	return origins
}

// ClassifyManaged는 버전 매니저 경로를 먼저 인식하고, 없으면 Classify로 넘어간다(순수 함수 — 테스트 대상).
func ClassifyManaged(resolved string, env TraceEnv) (manager, pkg string) {
	sep := string(filepath.Separator)
	has := func(dir string) bool { return strings.Contains(resolved, sep+dir+sep) }
	switch {
	case has(".nvm"):
		return MgrNvm, ""
	case has(".pyenv"):
		return MgrPyenv, ""
	case has(".asdf"):
		return MgrAsdf, ""
	case has(".rustup"):
		return MgrRustup, ""
	case has(".sdkman"):
		return MgrSdkman, ""
	case strings.Contains(resolved, sep+"mise"+sep):
		return MgrMise, ""
	}
	return Classify(resolved, env)
}

// Classify는 해소된 실제 경로만 보고 관리 주체와 소속 패키지를 판별한다(순수 함수 — 테스트 대상).
func Classify(resolved string, env TraceEnv) (manager, pkg string) {
	segs := strings.Split(resolved, string(filepath.Separator))

	// Homebrew: .../Cellar/<formula>/... 또는 .../Caskroom/<cask>/...
	if i := indexOf(segs, "Cellar"); i >= 0 && i+1 < len(segs) {
		return MgrBrew, segs[i+1]
	}
	if i := indexOf(segs, "Caskroom"); i >= 0 && i+1 < len(segs) {
		return MgrBrew, segs[i+1]
	}
	// npm: .../node_modules/<pkg> (스코프 패키지 @org/pkg 처리)
	if i := indexOf(segs, "node_modules"); i >= 0 && i+1 < len(segs) {
		p := segs[i+1]
		if strings.HasPrefix(p, "@") && i+2 < len(segs) {
			p = p + "/" + segs[i+2]
		}
		return MgrNpm, p
	}
	// pipx: .../pipx/venvs/<name>/...
	if indexOf(segs, "pipx") >= 0 {
		if j := indexOf(segs, "venvs"); j >= 0 && j+1 < len(segs) {
			return MgrPipx, segs[j+1]
		}
		return MgrPipx, ""
	}
	// cargo: ~/.cargo/bin (또는 CARGO_HOME)
	if strings.Contains(resolved, string(filepath.Separator)+".cargo"+string(filepath.Separator)) {
		return MgrCargo, ""
	}
	// go: $GOBIN 또는 ~/go/bin 아래
	if env.GoBin != "" && strings.HasPrefix(resolved, env.GoBin+string(filepath.Separator)) {
		return MgrGo, ""
	}
	// 시스템 경로
	for _, p := range []string{"/usr/bin/", "/bin/", "/usr/sbin/", "/sbin/", "/usr/libexec/", "/System/"} {
		if strings.HasPrefix(resolved, p) {
			return MgrSystem, ""
		}
	}
	return MgrManual, ""
}

func indexOf(s []string, x string) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}

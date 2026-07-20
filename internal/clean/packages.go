package clean

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

// 패키지 매니저로 설치된 항목의 타깃 종류.
const (
	KindNpm   = "npm-global"
	KindPipx  = "pipx"
	KindCargo = "cargo"
	KindGoBin = "go-bin"
)

// appendPackageTargets는 query가 npm/pipx/cargo 전역 패키지 또는 ~/go/bin 실행 파일이면
// 해당 삭제 타깃을 plan에 더한다(각 소스는 미설치면 조용히 스킵).
// 프로브 3개(각각 외부 명령 1회)는 독립이라 병렬 조회한다. go bin은 stat 1회라 동기.
func appendPackageTargets(ctx context.Context, run *iexec.Runner, plan *Plan, query, home string) {
	var (
		wg                    sync.WaitGroup
		npmOK, pipxOK, cargoOK bool
	)
	wg.Add(3)
	go func() { defer wg.Done(); npmOK = npmGlobalInstalled(ctx, run, query) }()
	go func() { defer wg.Done(); pipxOK = pipxInstalled(ctx, run, query) }()
	go func() { defer wg.Done(); cargoOK = cargoInstalled(ctx, run, query) }()
	wg.Wait()

	if npmOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindNpm, Name: query,
			Commands:    []string{"npm uninstall -g " + query},
			Suggestions: []string{"npm 전역 패키지는 ~/Library 잔여물이 거의 없습니다."},
		})
	}
	if pipxOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindPipx, Name: query,
			Commands: []string{"pipx uninstall " + query},
		})
	}
	if cargoOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindCargo, Name: query,
			Commands: []string{"cargo uninstall " + query},
		})
	}
	if bin := goBinPath(home, query); bin != "" && !IsProtected(bin) {
		// go는 uninstall 명령이 없으므로 바이너리를 휴지통으로.
		plan.Targets = append(plan.Targets, Target{
			Kind: KindGoBin, Name: query,
			TrashPaths: []PathMatch{{Path: bin, Confidence: ConfHigh, Reason: "go 설치 바이너리 (~/go/bin)"}},
		})
	}
}

func npmGlobalInstalled(ctx context.Context, run *iexec.Runner, name string) bool {
	// npm ls는 없는 패키지에 exit 1을 내므로 허용. 있으면 dependencies에 등장.
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60, OKCodes: []int{0, 1}},
		"npm", "ls", "-g", name, "--depth=0", "--json")
	if err != nil || strings.TrimSpace(out) == "" {
		return false
	}
	var parsed struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if json.Unmarshal([]byte(out), &parsed) != nil {
		return false
	}
	_, ok := parsed.Dependencies[name]
	return ok
}

func pipxInstalled(ctx context.Context, run *iexec.Runner, name string) bool {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "pipx", "list", "--json")
	if err != nil || strings.TrimSpace(out) == "" {
		return false
	}
	var parsed struct {
		Venvs map[string]json.RawMessage `json:"venvs"`
	}
	if json.Unmarshal([]byte(out), &parsed) != nil {
		return false
	}
	_, ok := parsed.Venvs[name]
	return ok
}

func cargoInstalled(ctx context.Context, run *iexec.Runner, name string) bool {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "cargo", "install", "--list")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		// "ripgrep v14.1.0:" 형식의 첫 토큰이 패키지명.
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

func goBinPath(home, name string) string {
	dir := os.Getenv("GOBIN")
	if dir == "" {
		dir = filepath.Join(home, "go", "bin")
	}
	p := filepath.Join(dir, name)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// 전역 설치 힌트: 지금 실행한 바이너리가 PATH 밖에 있으면(= go run이나 ./macinv처럼
// 프로젝트 경로에서 직접 실행) "어디서든 쓰려면 설치하라"고 한 줄 알린다.
//
// 안내 명령이 `go install .`인 이유: 이 저장소는 **private**이라 공개 모듈 프록시가
// 읽지 못해 `go install github.com/kyungryeol-yoon/macinv@latest`가 동작하지 않는다.
// 힌트가 뜨는 상황은 애초에 저장소 안에서 실행 중일 때라 `go install .`이 바로 먹힌다.
// (저장소를 public으로 전환하면 이 문구를 @latest 형태로 바꾸면 된다.)
const globalInstallHint = "  💡 어디서든 실행하려면: go install .   " +
	"(저장소가 private이라 `go install github.com/kyungryeol-yoon/macinv@latest`는 동작하지 않습니다)"

// attachInstallHint는 모든 커맨드가 끝난 뒤 힌트를 낼지 판단하는 훅을 단다.
func attachInstallHint(root *cobra.Command) {
	root.PersistentPostRun = func(cmd *cobra.Command, _ []string) {
		if shouldHintInstall(cmd, runningFromPATH()) {
			fmt.Fprintln(os.Stderr, globalInstallHint)
		}
	}
}

// shouldHintInstall은 이 실행에서 힌트를 낼지 판단한다(순수 함수 — 테스트 대상).
// 억제 조건: 이미 PATH에 있음 · TUI(대체 화면을 쓴다) · --json(파이프 오염) · --quiet(조용히 해달라는 뜻).
func shouldHintInstall(cmd *cobra.Command, onPATH bool) bool {
	if cmd == nil || onPATH || cmd.Name() == "tui" {
		return false
	}
	for _, flag := range []string{"json", "quiet"} {
		if f := cmd.Flags().Lookup(flag); f != nil && f.Value.String() == "true" {
			return false
		}
	}
	return true
}

// runningFromPATH는 지금 실행 중인 바이너리가 PATH 위에 있는지 본다.
// 판단이 애매하면(경로 조회 실패) true로 본다 — 확신 없이 잔소리하지 않는다.
func runningFromPATH() bool {
	exe, err := os.Executable()
	if err != nil {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return DirOnPATH(filepath.Dir(exe), os.Getenv("PATH"))
}

// DirOnPATH는 dir이 PATH 목록에 있는지 본다(순수 함수 — 테이블 테스트 대상).
// 빈 PATH 항목은 관례상 현재 디렉토리를 뜻하지만 여기서는 무시한다 —
// "." 때문에 설치된 것으로 오판하면 힌트가 영영 안 뜬다.
func DirOnPATH(dir, pathEnv string) bool {
	if dir == "" {
		return false
	}
	want := filepath.Clean(dir)
	for _, entry := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if entry == "" {
			continue
		}
		if filepath.Clean(entry) == want {
			return true
		}
	}
	return false
}

package clean

import (
	"context"
	"fmt"
	"os/exec"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

// TrashResult는 경로 하나의 휴지통 이동 결과다.
type TrashResult struct {
	Path string
	Err  error
}

// trashTool은 사용 가능한 휴지통 이동 방식을 반환한다(있으면 trash CLI 우선).
func trashTool() (string, bool) {
	if p, err := exec.LookPath("trash"); err == nil {
		return p, true
	}
	return "", false
}

// MoveToTrash는 경로를 휴지통으로 옮긴다. rm은 절대 쓰지 않는다.
// 안전 재확인: 블랙리스트 경로면 실행을 거부한다(계획 단계에서 걸러지지만 방어적으로 한 번 더).
func MoveToTrash(ctx context.Context, run *iexec.Runner, path string) error {
	if IsProtected(path) {
		return fmt.Errorf("보호된 시스템 경로 — 삭제 거부: %s", path)
	}
	if tool, ok := trashTool(); ok {
		_, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60, Strict: true}, tool, path)
		return err
	}
	// 폴백: Finder에게 휴지통 이동 위임(rm 아님).
	// 경로를 스크립트 본문에 끼워 넣지 않고 argv로 넘긴다 —
	// Go의 %q는 Go 이스케이프 규칙이라 AppleScript 문자열 규칙과 어긋날 수 있고,
	// on run argv 형태면 따옴표/역슬래시가 든 경로도 이스케이프 없이 그대로 전달된다.
	// `POSIX file <변수>`는 리터럴과 달리 alias 강제 변환이 필요하다(없으면 Finder -1728).
	_, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60, Strict: true}, "osascript",
		"-e", "on run argv",
		"-e", "set p to POSIX file (item 1 of argv) as alias",
		"-e", `tell application "Finder" to delete p`,
		"-e", "end run",
		"--", path)
	return err
}

// Execute는 계획을 실제로 수행한다(네이티브 명령 실행 + 휴지통 이동).
// 호출자는 반드시 --apply + 대화형 확인을 거친 뒤에만 이 함수를 불러야 한다.
// 각 단계 결과를 onCommand/onTrash 콜백으로 보고한다.
func Execute(ctx context.Context, run *iexec.Runner, plan *Plan,
	onCommand func(cmdline string, err error),
	onTrash func(res TrashResult),
) {
	for _, t := range plan.Targets {
		for _, argv := range t.Argv {
			err := runArgv(ctx, run, argv, t.CmdTimeoutSec)
			onCommand(ShellQuote(argv), err)
		}
		for _, pm := range t.TrashPaths {
			err := MoveToTrash(ctx, run, pm.Path)
			onTrash(TrashResult{Path: pm.Path, Err: err})
		}
	}
}

// runArgv는 계획의 명령(인자 배열)을 셸 없이 그대로 실행한다.
// 문자열로 저장했다가 공백으로 다시 쪼개지 않는 것이 핵심 — 공백이 든 인자가 살아남는다.
// timeoutSec가 0이면 기본 300초.
func runArgv(ctx context.Context, run *iexec.Runner, argv []string, timeoutSec int) error {
	if len(argv) == 0 {
		return nil
	}
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	// 변경 명령은 Strict: 비허용 종료코드는 stdout이 있어도 실패로 본다(부분 실패를 성공으로 오인 방지).
	_, err := run.Run(ctx, iexec.Opt{TimeoutSec: timeoutSec, Strict: true}, argv[0], argv[1:]...)
	return err
}

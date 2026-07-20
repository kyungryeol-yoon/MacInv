package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
	"github.com/kyungryeol-yoon/macinv/internal/tui"
)

func newTuiCmd() *cobra.Command {
	var noOutdated bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "대화형 TUI (space 멀티선택, d 삭제, r 재스캔, ←/→ 탭, ↑/↓ 이동, q 종료)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTui(cmd.Context(), noOutdated)
		},
	}
	cmd.Flags().BoolVar(&noOutdated, "no-outdated", false, "최신버전 체크 생략 (더 빠름)")
	return cmd
}

func runTui(ctx context.Context, noOutdated bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 120)

	fmt.Fprintln(os.Stderr, "  … 스캔 중 (TUI 준비, 잠시만요)")
	rep := collectInventory(ctx, run, rec, collectOpts{
		WithSize:    true,
		NoOutdated:  noOutdated,
		WithRuntime: true,
		Step: func(msg string) {
			fmt.Fprintf(os.Stderr, "  … %s\n", msg)
		},
	})
	// 시작 스캔도 캐시에 반영(scan 커맨드와 동일한 신선도 보장).
	if err := PersistReport(rep); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  캐시 저장 실패: %v\n", err)
	}

	// 부분 재스캔/캐시 기록 주입: 스캔 조립·파일 경로는 cmd, 화면은 tui (레이어 분리).
	rescan := func(rctx context.Context, sources []string) *report.Report {
		return RescanSources(rctx, run, rec, sources)
	}
	return tui.Run(ctx, run, rep, rescan, PersistReport)
}

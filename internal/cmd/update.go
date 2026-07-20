package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

func newUpdateCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "업데이트 플래너 (brew/npm/mas 최신 버전으로, 기본 dry-run)",
		Long: "brew formula/cask · npm 전역 · App Store(mas) 항목을 최신 버전으로 올리는 계획을 만든다.\n" +
			"clean과 같은 렌더러/실행기를 공유하며, 실제 실행은 --apply + 이름 확인이 있을 때만.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "실제로 업데이트 실행 (대화형 확인 필요)")
	return cmd
}

func runUpdate(ctx context.Context, name string, apply bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 60)

	fmt.Fprintln(os.Stderr, "  … 업데이트 계획 작성 중 (읽기 전용)")
	plan := clean.BuildUpgradePlan(ctx, run, name)

	fmt.Fprint(os.Stdout, "\n"+clean.RenderPlanText(plan))

	if !plan.HasContent() {
		fmt.Fprintf(os.Stdout, "\n  '%s'에 대해 업데이트할 대상을 찾지 못했습니다.\n\n", name)
		return nil
	}
	if !apply {
		fmt.Fprintf(os.Stdout, "\n  ℹ️  이건 dry-run입니다. 실제 업데이트하려면: macinv update %s --apply\n\n", name)
		return nil
	}

	fmt.Fprintf(os.Stdout, "\n  ⚠️  위 명령을 실행합니다.\n")
	fmt.Fprintf(os.Stdout, "  계속하려면 대상 이름 '%s'를 정확히 입력하세요 (취소: 그 외 아무 값): ", name)
	if !confirm(os.Stdin, name) {
		fmt.Fprintf(os.Stdout, "\n  취소되었습니다.\n\n")
		return nil
	}

	fmt.Fprintln(os.Stdout, "\n  실행 중…")
	clean.Execute(ctx, run, plan,
		func(cmdline string, err error) {
			if err != nil {
				fmt.Fprintf(os.Stdout, "    ✗ %s — 실패: %v\n", cmdline, err)
			} else {
				fmt.Fprintf(os.Stdout, "    ✓ %s\n", cmdline)
			}
		},
		func(res clean.TrashResult) {}, // 업데이트엔 휴지통 이동 없음
	)
	fmt.Fprintln(os.Stdout)
	return nil
}

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

func newUpdateCmd() *cobra.Command {
	var apply, all bool
	cmd := &cobra.Command{
		Use:   "update [<name>]",
		Short: "업데이트 플래너 (brew/npm/mas 최신 버전으로, 기본 dry-run, --all로 일괄)",
		Long: "brew formula/cask · npm 전역 · App Store(mas) 항목을 최신 버전으로 올리는 계획을 만든다.\n" +
			"--all이면 업데이트 대기 중인 전부를 한 번에. clean과 같은 렌더러/실행기를 공유하며,\n" +
			"실제 실행은 --apply + 확인 입력이 있을 때만.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) { // 둘 다 or 둘 다 아님
				return fmt.Errorf("<name> 또는 --all 중 하나만 지정하세요")
			}
			if all {
				return runUpdateAll(cmd.Context(), apply)
			}
			return runUpdate(cmd.Context(), args[0], apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "실제로 업데이트 실행 (대화형 확인 필요)")
	cmd.Flags().BoolVar(&all, "all", false, "업데이트 대기 중인 모든 항목(brew/npm/mas)을 일괄 업데이트")
	return cmd
}

// runUpdateAll은 outdated 전부를 한 계획으로 묶어 실행한다(확인 입력: all).
func runUpdateAll(ctx context.Context, apply bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 120)

	step := func(msg string) { fmt.Fprintf(os.Stderr, "  … %s\n", msg) }
	rows := collectOutdatedRows(ctx, run, rec, step)
	// 런타임 메이저는 --all에서 빼고 목록으로만 보여준다 — 아래 printHeldRuntimes 참고.
	auto, held := clean.SplitRuntimeMajors(rows)
	plan := clean.UpgradePlanFromOutdated(auto)

	if !plan.HasContent() {
		if len(held) > 0 {
			printHeldRuntimes(held)
			return nil
		}
		fmt.Fprintf(os.Stdout, "\n  ✅ 모든 패키지가 최신 상태입니다.\n\n")
		return nil
	}

	fmt.Fprint(os.Stdout, "\n"+clean.RenderPlanText(plan))
	printHeldRuntimes(held)
	if !apply {
		fmt.Fprintf(os.Stdout, "\n  ℹ️  이건 dry-run입니다. 실제 업데이트하려면: macinv update --all --apply\n\n")
		return nil
	}

	fmt.Fprintf(os.Stdout, "\n  ⚠️  위 %d개 항목을 전부 업데이트합니다.\n", len(plan.Targets))
	fmt.Fprintf(os.Stdout, "  계속하려면 'all'을 정확히 입력하세요 (취소: 그 외 아무 값): ")
	if !confirm(os.Stdin, "all") {
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
		func(res clean.TrashResult) {},
	)
	fmt.Fprintln(os.Stdout)
	return nil
}

// printHeldRuntimes는 --all에서 제외한 런타임 메이저를 목록과 명령으로만 보여준다.
// 실행하지 않는 이유를 함께 적는다 — 빠진 것을 사용자가 "누락"으로 오해하면 안 된다.
func printHeldRuntimes(held []report.OutdatedRow) {
	if len(held) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "\n  ⏸  런타임 메이저 업그레이드 %d건은 --all에서 제외했습니다"+
		" (프로젝트가 깨질 수 있어 직접 실행하세요):\n", len(held))
	for _, r := range held {
		fmt.Fprintf(os.Stdout, "     %-20s %s → %s\n", r.Name, r.Current, r.Latest)
		fmt.Fprintf(os.Stdout, "       $ brew upgrade %s\n", r.Name)
	}
	fmt.Fprintf(os.Stdout, "     현황 확인: macinv runtimes\n")
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

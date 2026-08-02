package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

func newCleanCmd() *cobra.Command {
	var apply, includeLow bool
	cmd := &cobra.Command{
		Use:   "clean <name>",
		Short: "삭제 플래너 (기본 dry-run, --apply + 확인 시에만 실제 삭제)",
		Long: "앱/brew 포뮬러/캐스크의 삭제 계획을 만든다. 잔여물은 Bundle ID 정확 일치를 우선하고,\n" +
			"실제 삭제는 rm이 아니라 휴지통 이동으로, --apply와 대화형 확인이 모두 있을 때만 수행한다.\n" +
			"삭제 시 기본은 [확실] 항목만 지우고, [확신낮음] 이름 기반 항목은 --include-low가 있을 때만 포함한다.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClean(cmd.Context(), args[0], apply, includeLow)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "실제로 삭제 실행 (대화형 확인 필요). 미지정 시 계획만 출력(dry-run)")
	cmd.Flags().BoolVar(&includeLow, "include-low", false, "삭제 시 [확신낮음](이름 기반) 항목도 포함")
	return cmd
}

func runClean(ctx context.Context, name string, apply, includeLow bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 60)

	fmt.Fprintln(os.Stderr, "  … 삭제 계획 작성 중 (읽기 전용)")
	plan := clean.BuildPlan(ctx, run, name)

	fmt.Fprint(os.Stdout, "\n"+clean.RenderPlanText(plan))

	if !plan.HasContent() {
		fmt.Fprintf(os.Stdout, "\n  '%s'에 대해 삭제할 대상을 찾지 못했습니다.\n\n", name)
		return nil
	}

	if !apply {
		if low := plan.LowConfidenceCount(); low > 0 {
			fmt.Fprintf(os.Stdout, "\n  ℹ️  [확신낮음] %d개는 --apply 시 기본 제외됩니다 (포함하려면 --include-low).\n", low)
		}
		fmt.Fprintf(os.Stdout, "\n  ℹ️  이건 dry-run입니다. 실제 삭제하려면: macinv clean %s --apply\n\n", name)
		return nil
	}

	// 실제 삭제: 기본은 [확실]만. --include-low가 없으면 낮은 확신 항목 제외.
	effective := plan
	if !includeLow {
		if skipped := plan.LowConfidenceCount(); skipped > 0 {
			effective = plan.WithoutLowConfidence()
			fmt.Fprintf(os.Stdout, "\n  🔒 [확신낮음] %d개는 이번 삭제에서 제외합니다 (포함하려면 --include-low).\n", skipped)
		}
	}

	// --apply: 대화형 확인 (경로 없이 삭제 못 하게 이름 정확 입력 요구).
	fmt.Fprintf(os.Stdout, "\n  ⚠️  위 항목을 휴지통으로 옮기고 네이티브 삭제 명령을 실행합니다.\n")
	fmt.Fprintf(os.Stdout, "  계속하려면 대상 이름 '%s'를 정확히 입력하세요 (취소: 그 외 아무 값): ", name)
	if !confirm(os.Stdin, name) {
		fmt.Fprintf(os.Stdout, "\n  취소되었습니다. 아무것도 삭제하지 않았습니다.\n\n")
		return nil
	}

	fmt.Fprintln(os.Stdout, "\n  실행 중…")
	clean.Execute(ctx, run, effective,
		func(cmdline string, err error) {
			if err != nil {
				fmt.Fprintf(os.Stdout, "    ✗ %s — 실패: %v\n", cmdline, err)
			} else {
				fmt.Fprintf(os.Stdout, "    ✓ %s\n", cmdline)
			}
		},
		func(res clean.TrashResult) {
			if res.Err != nil {
				fmt.Fprintf(os.Stdout, "    ✗ 휴지통 이동 실패: %s — %v\n", res.Path, res.Err)
			} else {
				fmt.Fprintf(os.Stdout, "    ✓ 휴지통으로 이동: %s\n", res.Path)
			}
		},
	)
	fmt.Fprintln(os.Stdout)
	return nil
}

// confirm은 입력에서 한 줄을 읽어 기대값과 정확히 일치하는지 본다.
func confirm(r *os.File, want string) bool {
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		return false
	}
	return strings.TrimSpace(sc.Text()) == want
}

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

func newOrphansCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "고아 잔여물 역스캔 — 이미 지운 앱들이 ~/Library에 남긴 데이터 정리 (기본 dry-run)",
		Long: "설치된 앱 Bundle ID 집합과 ~/Library 항목을 대조해, 어느 설치 앱에도 속하지 않는\n" +
			"Bundle ID 형태의 잔여 데이터를 찾는다. com.apple.*·설치 앱 파생물·벤더 공유 컴포넌트는\n" +
			"보존하고, 이름 폴더(Google, Notion 등)는 귀속을 확신할 수 없어 건드리지 않는다.\n" +
			"실제 삭제는 --apply + 대화형 확인 시에만, 휴지통 이동으로 수행한다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrphans(cmd.Context(), apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "실제로 정리 실행 (대화형 확인 필요). 미지정 시 목록만 출력(dry-run)")
	return cmd
}

func runOrphans(ctx context.Context, apply bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 60)

	fmt.Fprintln(os.Stderr, "  … 설치 앱 Bundle ID 수집 + ~/Library 역스캔 중 (읽기 전용)")
	plan := clean.BuildOrphansPlan(ctx, run)

	if !plan.HasContent() {
		fmt.Fprintf(os.Stdout, "\n  ✅ 고아 잔여물이 없습니다. 깨끗한 상태예요.\n\n")
		return nil
	}

	fmt.Fprint(os.Stdout, "\n"+clean.RenderPlanText(plan))
	paths := 0
	for _, t := range plan.Targets {
		paths += len(t.TrashPaths)
	}
	fmt.Fprintf(os.Stdout, "\n  발견: 고아 Bundle ID %d개, 경로 %d건\n", len(plan.Targets), paths)

	if !apply {
		fmt.Fprintf(os.Stdout, "\n  ℹ️  이건 dry-run입니다. 실제 정리하려면: macinv orphans --apply\n\n")
		return nil
	}

	fmt.Fprintf(os.Stdout, "\n  ⚠️  위 항목을 휴지통으로 옮깁니다(복구 가능).\n")
	fmt.Fprintf(os.Stdout, "  계속하려면 'orphans'를 정확히 입력하세요 (취소: 그 외 아무 값): ")
	if !confirm(os.Stdin, "orphans") {
		fmt.Fprintf(os.Stdout, "\n  취소되었습니다. 아무것도 삭제하지 않았습니다.\n\n")
		return nil
	}

	fmt.Fprintln(os.Stdout, "\n  실행 중…")
	clean.Execute(ctx, run, plan,
		func(cmdline string, err error) {},
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

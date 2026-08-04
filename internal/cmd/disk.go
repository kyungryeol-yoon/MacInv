package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	"github.com/kyungryeol-yoon/macinv/internal/disk"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

type diskFlags struct {
	jsonOut          bool
	apply            bool
	includeExpensive bool
	includeRunning   bool
	deep             bool
	quiet            bool
}

func newDiskCmd() *cobra.Command {
	var f diskFlags
	cmd := &cobra.Command{
		Use:   "disk",
		Short: "디스크를 뭐가 먹고 있는지 (캐시·모델·빌드 산출물, 기본 dry-run)",
		Long: "알려진 캐시/데이터 위치를 용량순으로 보여준다. 인벤토리와 대조해 '도구는 없는데\n" +
			"캐시만 남은' 고아 캐시를 표시하고, APFS 로컬 스냅샷 때문에 지워도 용량이 안 늘어나는\n" +
			"상황을 안내한다. 정리는 rm이 아니라 네이티브 명령(brew cleanup 등)과 휴지통 이동으로,\n" +
			"--apply + 대화형 확인이 있을 때만 수행한다. 재다운로드가 비싼 항목(모델 등)은\n" +
			"--include-expensive가 있을 때만 포함하고, 실행 중인 앱의 캐시는 기본 제외한다\n" +
			"(Chromium은 캐시 핸들을 연 채로 돌아 오작동할 수 있다 — --include-running으로 해제).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisk(cmd.Context(), f)
		},
	}
	cmd.Flags().BoolVar(&f.jsonOut, "json", false, "JSON을 표준출력으로 출력")
	cmd.Flags().BoolVar(&f.apply, "apply", false, "실제로 정리 실행 (대화형 확인 필요). 미지정 시 계획만 출력(dry-run)")
	cmd.Flags().BoolVar(&f.includeExpensive, "include-expensive", false, "재다운로드가 비싼 항목(모델·이미지)도 정리 대상에 포함")
	cmd.Flags().BoolVar(&f.includeRunning, "include-running", false, "실행 중인 앱의 캐시도 정리 대상에 포함 (앱이 오작동할 수 있음)")
	cmd.Flags().BoolVar(&f.deep, "deep", false, "홈 디렉토리에서 오래된 node_modules까지 탐색 (느림)")
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "진행 표시 생략")
	return cmd
}

func runDisk(ctx context.Context, f diskFlags) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 120)
	home := disk.UserHome()

	step := func(msg string) {
		if !f.quiet && !f.jsonOut {
			fmt.Fprintf(os.Stderr, "  … %s\n", msg)
		}
	}

	// 고아 캐시 판정에는 인벤토리가 필요하다. 캐시가 있으면 그걸 쓰고(빠름),
	// 없으면 용량/네트워크를 뺀 가벼운 스캔을 한 번 돈다.
	step("인벤토리 대조 준비 중")
	tools := disk.DetectTools(loadOrScanInventory(ctx, run, rec))

	step("알려진 위치 용량 측정 중")
	res := disk.Scan(ctx, run, home, tools)

	step("APFS 로컬 스냅샷 확인 중")
	res.Snapshots = disk.ScanSnapshots(ctx, run)

	if f.deep {
		step("홈 디렉토리에서 오래된 node_modules 탐색 중 (느립니다)")
		res.Deep = disk.ScanDeep(ctx, run, home, time.Now())
	}

	if f.jsonOut {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	disk.PrintSummary(os.Stdout, res, f.includeExpensive)

	plan := disk.BuildCleanupPlan(res, nil, f.includeExpensive, f.includeRunning)
	if !plan.HasContent() {
		fmt.Fprintf(os.Stdout, "\n  정리할 수 있는 항목이 없습니다.\n\n")
		return nil
	}
	fmt.Fprint(os.Stdout, "\n"+clean.RenderPlanText(plan))

	if !f.apply {
		fmt.Fprintf(os.Stdout, "\n  ℹ️  이건 dry-run입니다. 실제 정리하려면: macinv disk --apply\n")
		if res.ExpensiveKB > 0 && !f.includeExpensive {
			fmt.Fprintf(os.Stdout, "  ℹ️  재다운로드가 비싼 항목은 --include-expensive가 있어야 포함됩니다.\n")
		}
		fmt.Fprintln(os.Stdout)
		return nil
	}

	fmt.Fprintf(os.Stdout, "\n  ⚠️  위 명령을 실행하고 표시된 경로를 휴지통으로 옮깁니다.\n")
	fmt.Fprintf(os.Stdout, "  계속하려면 'disk'를 정확히 입력하세요 (취소: 그 외 아무 값): ")
	if !confirm(os.Stdin, "disk") {
		fmt.Fprintf(os.Stdout, "\n  취소되었습니다. 아무것도 정리하지 않았습니다.\n\n")
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
		func(r clean.TrashResult) {
			if r.Err != nil {
				fmt.Fprintf(os.Stdout, "    ✗ 휴지통 이동 실패: %s — %v\n", r.Path, r.Err)
			} else {
				fmt.Fprintf(os.Stdout, "    ✓ 휴지통으로 이동: %s\n", r.Path)
			}
		},
	)

	printOllamaHint(ctx, run)
	fmt.Fprintln(os.Stdout)
	return nil
}

// printOllamaHint는 ollama 모델을 개별 선택해 지우도록 안내한다.
// 모델은 하나가 수 GB이고 재다운로드가 비싸 위치 단위로 뭉뚱그려 지우면 안 된다.
func printOllamaHint(ctx context.Context, run *iexec.Runner) {
	models := disk.ScanOllamaModels(ctx, run)
	if len(models) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "\n  🦙 Ollama 모델은 개별 선택으로만 지웁니다:\n")
	for _, m := range models {
		kb := m.SizeKB
		fmt.Fprintf(os.Stdout, "    %9s  %-28s → ollama rm %s\n", report.HumanSize(&kb), m.Name, m.Name)
	}
}

// loadOrScanInventory는 캐시된 인벤토리를 읽고, 없으면 가벼운 스캔을 한 번 돈다.
// 고아 캐시 판정은 "이 도구가 설치돼 있나"만 알면 되므로 용량/최신버전 조회는 건너뛴다.
func loadOrScanInventory(ctx context.Context, run *iexec.Runner, rec *iexec.Recorder) *report.Report {
	if data, err := os.ReadFile(defaultCachePath()); err == nil {
		var rep report.Report
		// runtimes가 없는 캐시는 --no-runtimes로 만들어졌거나 오래된 것이다.
		// 그대로 쓰면 python/go가 없다고 보고 pip·go 캐시를 고아로 오탐한다.
		if json.Unmarshal(data, &rep) == nil && len(rep.Runtimes) > 0 {
			return &rep
		}
	}
	return collectInventory(ctx, run, rec, collectOpts{
		WithSize: false, NoOutdated: true, WithRuntime: true,
	})
}

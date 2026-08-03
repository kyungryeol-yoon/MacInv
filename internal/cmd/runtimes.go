package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/source"
)

func newRuntimesCmd() *cobra.Command {
	var jsonOut, offline bool
	cmd := &cobra.Command{
		Use:   "runtimes",
		Short: "개발 언어/런타임 현황 (버전·관리 주체·지원 종료(EOL)·업데이트 상태)",
		Long: "설치된 런타임의 활성 버전·경로·관리 주체에 더해, endoflife.date로 지원 종료일을 조회해\n" +
			"[EOL 지남]/[N개월 뒤 종료]/[지원 중]을 표시한다. 업데이트는 [현재]/[안전한 패치 있음]/\n" +
			"[메이저 최신은 X] 세 갈래로 구분한다 — 런타임 메이저 업그레이드는 프로젝트를 깨뜨리므로\n" +
			"macinv는 명령을 보여주기만 하고 실행하지 않는다. 조회 결과는 7일 캐시되고,\n" +
			"오프라인이거나 조회에 실패하면 조용히 생략하고 설치 현황만 보여준다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRuntimes(cmd.Context(), jsonOut, offline)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON을 표준출력으로 출력")
	cmd.Flags().BoolVar(&offline, "offline", false, "지원 종료(EOL) 조회 생략 (캐시가 있으면 캐시만 사용)")
	return cmd
}

func runRuntimes(ctx context.Context, jsonOut, offline bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 30)
	env := source.CurrentTraceEnv()

	if !jsonOut {
		fmt.Fprintln(os.Stderr, "  … 런타임 병렬 조회 중")
	}
	runtimes := source.ScanRuntimes(ctx, run, env)
	managers := source.ScanVersionManagers(env)

	if !jsonOut && !offline {
		fmt.Fprintln(os.Stderr, "  … 지원 종료(EOL) 정보 확인 중 (7일 캐시)")
	}
	now := time.Now()
	source.EnrichEOL(ctx, runtimes, managers, offline, now)

	if jsonOut {
		data, err := json.MarshalIndent(struct {
			Runtimes        []source.Runtime        `json:"runtimes"`
			VersionManagers []source.VersionManager `json:"version_managers"`
		}{runtimes, managers}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	printRuntimesTable(runtimes, now)
	printVersionManagers(managers)
	return nil
}

func printRuntimesTable(runtimes []source.Runtime, now time.Time) {
	line := strings.Repeat("─", 78)
	fmt.Fprintf(os.Stdout, "\n%s\n 🧰 개발 런타임 현황\n%s\n", line, line)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  런타임\t활성 버전\t관리 주체\t지원 종료\t업데이트")
	anySuggestion := false
	for _, rt := range runtimes {
		if !rt.Installed {
			fmt.Fprintf(tw, "  %s\t미설치\t \t \t \n", rt.Name)
			continue
		}
		active := activeOrigin(rt.Origins)
		ver := rt.Version
		if ver == "" {
			ver = "?"
		}
		mgr := managerLabel(active.Manager)
		if rt.Conflict {
			mgr += fmt.Sprintf(" ⚠️ %d경로", source.CountResolved(rt.Origins))
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", rt.Name, ver, mgr,
			dash(source.EOLBadge(rt.EOLStatus, rt.EOLDate, now)),
			dash(source.UpdateBadge(rt.UpdateStatus, rt.LatestCycle)))
		if rt.SuggestedCommand != "" && needsUpgrade(rt.UpdateStatus) {
			anySuggestion = true
		}
	}
	tw.Flush()

	printSuggestions(runtimes, anySuggestion)
}

// printSuggestions는 올릴 때 쓸 명령만 보여준다. **실행하지 않는다** —
// 런타임 메이저 업그레이드는 프로젝트를 깨뜨리므로 자동 실행 대상이 아니다.
func printSuggestions(runtimes []source.Runtime, any bool) {
	if !any {
		return
	}
	fmt.Fprintf(os.Stdout, "\n  업데이트가 필요하면 (macinv는 실행하지 않습니다):\n")
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	for _, rt := range runtimes {
		if !rt.Installed || rt.SuggestedCommand == "" || !needsUpgrade(rt.UpdateStatus) {
			continue
		}
		note := ""
		if rt.UpdateStatus == source.UpdateMajor {
			note = "  ← 메이저 업그레이드: 프로젝트가 깨질 수 있습니다"
		}
		fmt.Fprintf(tw, "    %s\t$ %s%s\n", rt.Name, rt.SuggestedCommand, note)
	}
	tw.Flush()
}

// needsUpgrade는 실제로 더 새 버전이 있다고 확인된 상태인지 본다.
// 상태를 모를 때(조회 실패·대상 아님) 명령을 권하지 않는다 — 근거 없는 권유는 소음이다.
func needsUpgrade(status string) bool {
	return status == source.UpdatePatch || status == source.UpdateMajor
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printVersionManagers(managers []source.VersionManager) {
	if len(managers) == 0 {
		fmt.Fprintln(os.Stdout)
		return
	}
	line := strings.Repeat("─", 62)
	fmt.Fprintf(os.Stdout, "\n%s\n 🔀 버전 매니저 (관리 중인 버전)\n%s\n", line, line)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	for _, m := range managers {
		fmt.Fprintf(tw, "  %s\t%s\n", m.Name, strings.Join(m.Versions, ", "))
	}
	tw.Flush()
	fmt.Fprintln(os.Stdout)
}

func activeOrigin(origins []source.Origin) source.Origin {
	for _, o := range origins {
		if o.Active {
			return o
		}
	}
	if len(origins) > 0 {
		return origins[0]
	}
	return source.Origin{}
}

// managerLabel은 관리 주체를 한국어 라벨로 바꾼다(버전 매니저 포함).
func managerLabel(mgr string) string {
	switch mgr {
	case source.MgrBrew:
		return "brew"
	case source.MgrNvm:
		return "nvm"
	case source.MgrPyenv:
		return "pyenv"
	case source.MgrMise:
		return "mise"
	case source.MgrAsdf:
		return "asdf"
	case source.MgrRustup:
		return "rustup"
	case source.MgrSdkman:
		return "sdkman"
	case source.MgrGo:
		return "go"
	case source.MgrSystem:
		return "시스템"
	default:
		return "수동 설치"
	}
}

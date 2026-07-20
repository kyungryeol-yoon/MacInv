package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/source"
)

func newRuntimesCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "runtimes",
		Short: "개발 언어/런타임 현황 (활성 버전·경로·관리 주체, PATH 충돌 표시)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRuntimes(cmd.Context(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON을 표준출력으로 출력")
	return cmd
}

func runRuntimes(ctx context.Context, jsonOut bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 30)
	env := source.CurrentTraceEnv()

	if !jsonOut {
		fmt.Fprintln(os.Stderr, "  … 런타임 병렬 조회 중")
	}
	runtimes := source.ScanRuntimes(ctx, run, env)
	managers := source.ScanVersionManagers(env)

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

	printRuntimesTable(runtimes)
	printVersionManagers(managers)
	return nil
}

func printRuntimesTable(runtimes []source.Runtime) {
	line := strings.Repeat("─", 62)
	fmt.Fprintf(os.Stdout, "\n%s\n 🧰 개발 런타임 현황\n%s\n", line, line)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  런타임\t활성 버전\t관리 주체\t경로")
	for _, rt := range runtimes {
		if !rt.Installed {
			fmt.Fprintf(tw, "  %s\t미설치\t \t \n", rt.Name)
			continue
		}
		active := activeOrigin(rt.Origins)
		path := active.Path
		if rt.Conflict {
			path = fmt.Sprintf("%s  ⚠️ %d개 경로", active.Path, source.CountResolved(rt.Origins))
		}
		ver := rt.Version
		if ver == "" {
			ver = "?"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", rt.Name, ver, managerLabel(active.Manager), path)
	}
	tw.Flush()
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

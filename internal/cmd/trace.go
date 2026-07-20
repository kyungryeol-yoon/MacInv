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

func newTraceCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "trace <name>",
		Short: "바이너리 소유권 역추적 (brew/npm/cargo/pipx/go/system/수동)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrace(cmd.Context(), args[0], jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON을 표준출력으로 출력")
	return cmd
}

func runTrace(ctx context.Context, name string, jsonOut bool) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 30)

	origins := source.Trace(ctx, run, name, source.CurrentTraceEnv())

	if jsonOut {
		if origins == nil {
			origins = []source.Origin{}
		}
		data, err := json.MarshalIndent(struct {
			Name    string          `json:"name"`
			Count   int             `json:"count"`
			Origins []source.Origin `json:"origins"`
		}{name, len(origins), origins}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	if len(origins) == 0 {
		fmt.Fprintf(os.Stdout, "\n  🔍 '%s' 을(를) PATH에서 찾을 수 없습니다.\n\n", name)
		return nil
	}

	line := strings.Repeat("─", 62)
	fmt.Fprintf(os.Stdout, "\n%s\n 🔍 '%s' 추적 결과 (%d개 경로)\n%s\n", line, name, len(origins), line)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  활성\t경로\t소유")
	for _, o := range origins {
		active := " "
		if o.Active {
			active = "★"
		}
		path := o.Path
		if o.Resolved != o.Path {
			path = o.Path + " → " + o.Resolved
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", active, path, originLabel(o))
	}
	tw.Flush()
	fmt.Fprintln(os.Stdout)
	return nil
}

// originLabel은 관리 주체를 한국어 라벨로 바꾼다.
func originLabel(o source.Origin) string {
	switch o.Manager {
	case source.MgrBrew:
		return "brew: " + o.Package
	case source.MgrNpm:
		return "npm: " + o.Package
	case source.MgrPipx:
		if o.Package != "" {
			return "pipx: " + o.Package
		}
		return "pipx"
	case source.MgrCargo:
		return "cargo"
	case source.MgrGo:
		return "go (~/go/bin)"
	case source.MgrSystem:
		return "시스템"
	default:
		return "수동 설치"
	}
}

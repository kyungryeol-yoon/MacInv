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
	"github.com/kyungryeol-yoon/macinv/internal/report"
	"github.com/kyungryeol-yoon/macinv/internal/source"
)

type outdatedFlags struct {
	jsonOut bool
}

func newOutdatedCmd() *cobra.Command {
	var f outdatedFlags
	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "brew/npm/mas 업데이트 대기 목록 (현재 → 최신)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutdated(cmd.Context(), f)
		},
	}
	cmd.Flags().BoolVar(&f.jsonOut, "json", false, "JSON을 표준출력으로 출력")
	return cmd
}

// collectOutdatedRows는 brew/npm/mas만 스캔·보강해 업데이트 대기 행을 모은다(outdated/update --all 공용).
func collectOutdatedRows(ctx context.Context, run *iexec.Runner, rec *iexec.Recorder, step func(string)) []report.OutdatedRow {
	step("brew/npm/mas 스캔 중")
	results := source.ScanAll(ctx, source.OutdatedSources(run), rec)
	sections := buildSections(results)

	if sections.Brew != nil {
		step("brew 최신버전 확인 중 (수십 초 걸릴 수 있음)")
		source.EnrichBrew(ctx, run, sections.Brew)
	}
	if sections.Npm != nil {
		step("npm 최신버전 확인 중")
		source.EnrichNpm(ctx, run, sections.Npm)
	}
	return report.OutdatedRows(&report.Report{Sections: sections})
}

func runOutdated(ctx context.Context, f outdatedFlags) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 120)

	step := func(msg string) {
		if !f.jsonOut {
			fmt.Fprintf(os.Stderr, "  … %s\n", msg)
		}
	}
	rows := collectOutdatedRows(ctx, run, rec, step)
	rep := &report.Report{GeneratedAt: time.Now().Format(time.RFC3339)}

	if f.jsonOut {
		payload := struct {
			GeneratedAt string               `json:"generated_at"`
			Count       int                  `json:"count"`
			Outdated    []report.OutdatedRow `json:"outdated"`
		}{rep.GeneratedAt, len(rows), rows}
		if rows == nil {
			payload.Outdated = []report.OutdatedRow{}
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	printOutdatedTable(os.Stdout, rows)
	return nil
}

// printOutdatedTable은 tabwriter로 정렬된 업데이트 대기 표를 출력한다.
func printOutdatedTable(w *os.File, rows []report.OutdatedRow) {
	line := strings.Repeat("─", 62)
	if len(rows) == 0 {
		fmt.Fprintf(w, "\n  ✅ 모든 패키지가 최신 상태입니다.\n\n")
		return
	}
	fmt.Fprintf(w, "\n%s\n ⬆️  업데이트 대기 중 (%d개)\n%s\n", line, len(rows), line)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  소스\t이름\t현재\t \t최신")
	for _, r := range rows {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t→\t%s\n", r.Source, r.Name, r.Current, r.Latest)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

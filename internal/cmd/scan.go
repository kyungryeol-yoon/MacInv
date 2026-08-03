package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
	"github.com/kyungryeol-yoon/macinv/internal/source"
)

type scanFlags struct {
	jsonOut    bool
	noOutdated bool
	noSizes    bool
	noRuntimes bool
	quiet      bool
	output     string
}

// collectOpts는 인벤토리 수집 동작 옵션이다(scan/tui 공용).
type collectOpts struct {
	WithSize    bool
	NoOutdated  bool
	WithRuntime bool
	Step        func(string)
	// Progress는 오래 걸리는 단계의 진행률 콜백이다(nil이면 표시하지 않음).
	Progress func(stage string, done, total int)
}

// collectInventory는 전체 인벤토리를 수집해 report.Report로 조립한다(scan/tui 공용, 출력은 하지 않음).
func collectInventory(ctx context.Context, run *iexec.Runner, rec *iexec.Recorder, opt collectOpts) *report.Report {
	step := opt.Step
	if step == nil {
		step = func(string) {}
	}

	step("전체 소스 병렬 스캔 중")
	results := source.ScanAll(ctx, source.Default(run, source.Options{
		WithSize: opt.WithSize,
		Progress: opt.Progress,
	}), rec)
	sections := buildSections(results)

	if !opt.NoOutdated {
		if sections.Brew != nil {
			step("brew 최신버전 확인 중 (수십 초 걸릴 수 있음)")
			source.EnrichBrew(ctx, run, sections.Brew)
		}
		if sections.Npm != nil {
			step("npm 최신버전 확인 중")
			source.EnrichNpm(ctx, run, sections.Npm)
		}
	}

	var runtimes []report.Runtime
	var managers []report.VersionManager
	if opt.WithRuntime {
		step("개발 런타임 조회 중")
		env := source.CurrentTraceEnv()
		runtimes = source.ScanRuntimes(ctx, run, env)
		managers = source.ScanVersionManagers(env)
		// EOL 배지는 **캐시만** 써서 채운다(offline=true). scan은 자주 도는 경로라
		// 네트워크 왕복을 얹지 않는다 — 캐시는 `macinv runtimes`가 채운다.
		source.EnrichEOL(ctx, runtimes, managers, true, time.Now())
	}

	host, _ := os.Hostname()
	rep := &report.Report{
		GeneratedAt:     time.Now().Format(time.RFC3339),
		Hostname:        host,
		OutdatedSkipped: opt.NoOutdated,
		Sections:        sections,
		Runtimes:        runtimes,
		VersionManagers: managers,
		Errors:          rec.Messages(),
	}
	if rep.Errors == nil {
		rep.Errors = []string{}
	}
	return rep
}

// progressPrinter는 진행률을 stderr 한 줄에 덮어써 갱신한다(\r). quiet면 아무것도 하지 않는다.
// stderr로 보내는 이유: --json의 stdout을 오염시키면 파이프가 깨진다.
// stderr가 터미널이 아니면(로그 파일 등) 커서 제어가 쓰레기 문자로 남으므로 완료 줄만 남긴다.
func progressPrinter(quiet bool) func(stage string, done, total int) {
	if quiet {
		return nil
	}
	tty := isTerminal(os.Stderr)
	var mu sync.Mutex
	return func(stage string, done, total int) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case done >= total:
			if tty {
				fmt.Fprint(os.Stderr, "\r\033[K")
			}
			fmt.Fprintf(os.Stderr, "  … %s %d/%d — 완료\n", stage, done, total)
		case tty:
			fmt.Fprintf(os.Stderr, "\r\033[K  … %s %d/%d", stage, done, total)
		}
	}
}

// isTerminal은 f가 터미널인지 본다(커서 제어 문자를 써도 되는지 판단).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func newScanCmd() *cobra.Command {
	var f scanFlags
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "전체 스캔 → 요약 테이블 (--json 시 JSON 출력)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context(), f)
		},
	}
	cmd.Flags().BoolVar(&f.jsonOut, "json", false, "JSON을 표준출력으로 출력")
	cmd.Flags().BoolVar(&f.noOutdated, "no-outdated", false, "최신버전 체크 생략 (네트워크 조회 없음, 빠름)")
	cmd.Flags().BoolVar(&f.noSizes, "no-sizes", false, "앱 용량(du) 계산 생략")
	cmd.Flags().BoolVar(&f.noRuntimes, "no-runtimes", false, "개발 런타임(언어 버전) 조회 생략")
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "요약 출력 생략")
	cmd.Flags().StringVarP(&f.output, "output", "o", "", "JSON 리포트 저장 경로 (기본: ~/.macinv/inventory.json)")
	return cmd
}

func runScan(ctx context.Context, f scanFlags) error {
	rec := &iexec.Recorder{}
	run := iexec.NewRunner(rec, 120)

	rep := collectInventory(ctx, run, rec, collectOpts{
		WithSize:    !f.noSizes,
		NoOutdated:  f.noOutdated,
		WithRuntime: !f.noRuntimes,
		Step: func(msg string) {
			if !f.quiet {
				fmt.Fprintf(os.Stderr, "  … %s\n", msg)
			}
		},
		Progress: progressPrinter(f.quiet),
	})

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}

	if f.jsonOut {
		fmt.Fprintln(os.Stdout, string(data))
	}

	outPath := f.output
	if outPath == "" {
		outPath = defaultCachePath()
	}
	if err := writeFile(outPath, data); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  리포트 저장 실패: %v\n", err)
	}

	if !f.quiet && !f.jsonOut {
		report.PrintSummary(os.Stdout, rep)
	}
	if !f.jsonOut {
		abs, _ := filepath.Abs(outPath)
		fmt.Fprintf(os.Stdout, "📄 상세 리포트: %s\n", abs)
	}
	return nil
}

// PersistReport는 리포트를 기본 캐시 경로(~/.macinv/inventory.json)에 기록한다(TUI 동기화용).
// GeneratedAt을 기록 시점으로 갱신해 캐시의 신선도를 반영한다.
func PersistReport(rep *report.Report) error {
	rep.GeneratedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(defaultCachePath(), data)
}

// RescanSources는 지정한 소스만 다시 스캔해 그 섹션이 채워진 report를 돌려준다(TUI 부분 재스캔용).
// names가 비면 전체 스캔과 동일. brew/npm은 outdated 보강까지 한다.
func RescanSources(ctx context.Context, run *iexec.Runner, rec *iexec.Recorder, names []string) *report.Report {
	all := source.Default(run, source.Options{WithSize: true})
	var sel []source.Source
	if len(names) == 0 {
		sel = all
	} else {
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		for _, s := range all {
			if want[s.Name()] {
				sel = append(sel, s)
			}
		}
	}
	results := source.ScanAll(ctx, sel, rec)
	sections := buildSections(results)
	if sections.Brew != nil {
		source.EnrichBrew(ctx, run, sections.Brew)
	}
	if sections.Npm != nil {
		source.EnrichNpm(ctx, run, sections.Npm)
	}
	return &report.Report{Sections: sections}
}

// buildSections는 ScanAll 결과 맵을 타입 있는 Sections로 조립한다.
func buildSections(m map[string]any) report.Sections {
	var s report.Sections
	if v, ok := m[source.NameApps].([]report.App); ok {
		s.Apps = v
	}
	if s.Apps == nil {
		s.Apps = []report.App{}
	}
	if v, ok := m[source.NameBrew].(*report.Brew); ok {
		s.Brew = v
	}
	if v, ok := m[source.NameNpm].(*report.Npm); ok {
		s.Npm = v
	}
	if v, ok := m[source.NamePipx].([]report.NameVersion); ok {
		s.Pipx = v
	}
	if v, ok := m[source.NameCargo].([]report.NameVersion); ok {
		s.Cargo = v
	}
	if v, ok := m[source.NameMas].(*report.Mas); ok {
		s.Mas = v
	}
	if v, ok := m[source.NameGoBin].([]report.GoBinItem); ok {
		s.GoBin = v
	}
	if v, ok := m[source.NameLaunchd].([]report.LaunchdItem); ok {
		s.Launchd = v
	}
	if s.Launchd == nil {
		s.Launchd = []report.LaunchdItem{}
	}
	return s
}

func defaultCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".macinv", "inventory.json")
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

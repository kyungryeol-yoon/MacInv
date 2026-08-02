package source

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// launchdSource는 LaunchAgents/LaunchDaemons plist를 스캔한다(백그라운드 잔존물 후보).
type launchdSource struct {
	run *iexec.Runner
}

func (s *launchdSource) Name() string    { return NameLaunchd }
func (s *launchdSource) Available() bool { return true }

type launchdDir struct {
	path string
	kind string
}

func launchdDirs() []launchdDir {
	home, _ := os.UserHomeDir()
	return []launchdDir{
		{filepath.Join(home, "Library", "LaunchAgents"), "user_agent"},
		{"/Library/LaunchAgents", "system_agent"},
		{"/Library/LaunchDaemons", "system_daemon"},
	}
}

func (s *launchdSource) Scan(ctx context.Context) (any, error) {
	items := []report.LaunchdItem{}
	for _, d := range launchdDirs() {
		matches, err := filepath.Glob(filepath.Join(d.path, "*.plist"))
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, pl := range matches {
			items = append(items, s.parseOne(ctx, pl, d.kind))
		}
	}
	return items, nil
}

// plist는 binary/xml 둘 다 가능하므로 plutil로 JSON 변환 후 파싱한다.
func (s *launchdSource) parseOne(ctx context.Context, file, kind string) report.LaunchdItem {
	item := report.LaunchdItem{File: file, Kind: kind}
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 20}, "plutil", "-convert", "json", "-o", "-", file)
	if err != nil {
		item.ParseError = report.Ptr(err.Error())
		return item
	}
	label, prog, runAtLoad, perr := parseLaunchdJSON([]byte(out))
	if perr != nil {
		item.ParseError = report.Ptr(perr.Error())
		return item
	}
	item.Label = label
	item.Program = prog
	item.RunAtLoad = runAtLoad
	return item
}

type launchdRaw struct {
	Label            *string  `json:"Label"`
	Program          *string  `json:"Program"`
	ProgramArguments []string `json:"ProgramArguments"`
	RunAtLoad        *bool    `json:"RunAtLoad"`
}

// parseLaunchdJSON은 plutil이 뽑은 JSON에서 Label/Program/RunAtLoad를 뽑는다(exec와 분리 — 테스트 대상).
func parseLaunchdJSON(data []byte) (label, program *string, runAtLoad *bool, err error) {
	var raw launchdRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, nil, err
	}
	prog := raw.Program
	if prog == nil && len(raw.ProgramArguments) > 0 {
		prog = report.Ptr(raw.ProgramArguments[0])
	}
	return raw.Label, prog, raw.RunAtLoad, nil
}

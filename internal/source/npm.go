package source

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// npmSource는 npm 글로벌 패키지를 스캔한다.
type npmSource struct {
	run *iexec.Runner
}

func (s *npmSource) Name() string   { return NameNpm }
func (s *npmSource) Available() bool { _, err := exec.LookPath("npm"); return err == nil }

func (s *npmSource) Scan(ctx context.Context) (any, error) {
	root, _ := s.run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "npm", "root", "-g")
	npm := &report.Npm{
		Root:     report.PtrOrNil(strings.TrimSpace(root)),
		Packages: []report.NpmPackage{},
	}
	// npm ls는 문제가 있어도 exit 1을 낼 수 있으므로 허용.
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 120, OKCodes: []int{0, 1}}, "npm", "ls", "-g", "--depth=0", "--json")
	if err != nil {
		return npm, nil
	}
	npm.Packages = parseNpmLs([]byte(out))
	return npm, nil
}

type npmLsRaw struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// parseNpmLs는 npm ls -g --json을 파싱한다(exec와 분리 — 테스트 대상).
func parseNpmLs(data []byte) []report.NpmPackage {
	var raw npmLsRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return []report.NpmPackage{}
	}
	pkgs := make([]report.NpmPackage, 0, len(raw.Dependencies))
	for name, meta := range raw.Dependencies {
		pkgs = append(pkgs, report.NpmPackage{
			Name:    name,
			Version: report.PtrOrNil(meta.Version),
		})
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs
}

// EnrichNpm은 npm outdated -g --json 결과로 outdated/latest_version을 채운다.
func EnrichNpm(ctx context.Context, run *iexec.Runner, npm *report.Npm) {
	// outdated 패키지가 있으면 exit 1이 정상.
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 180, OKCodes: []int{0, 1}}, "npm", "outdated", "-g", "--json")
	if err != nil {
		return
	}
	applyNpmOutdated([]byte(out), npm)
}

// applyNpmOutdated는 npm outdated JSON을 파싱해 npm에 반영한다(exec와 분리 — 테스트 대상).
func applyNpmOutdated(data []byte, npm *report.Npm) {
	if strings.TrimSpace(string(data)) == "" {
		return
	}
	var raw map[string]struct {
		Latest string `json:"latest"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	idx := map[string]int{}
	for i := range npm.Packages {
		idx[npm.Packages[i].Name] = i
	}
	for name, meta := range raw {
		if i, ok := idx[name]; ok {
			npm.Packages[i].Outdated = true
			npm.Packages[i].LatestVersion = report.PtrOrNil(meta.Latest)
		}
	}
}

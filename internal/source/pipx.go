package source

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// pipxSource는 pipx 설치 앱을 스캔한다.
type pipxSource struct {
	run *iexec.Runner
}

func (s *pipxSource) Name() string    { return NamePipx }
func (s *pipxSource) Available() bool { _, err := exec.LookPath("pipx"); return err == nil }

func (s *pipxSource) Scan(ctx context.Context) (any, error) {
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "pipx", "list", "--json")
	if err != nil {
		return []report.NameVersion{}, nil
	}
	return parsePipxList([]byte(out)), nil
}

type pipxRaw struct {
	Venvs map[string]struct {
		Metadata struct {
			MainPackage struct {
				PackageVersion string `json:"package_version"`
			} `json:"main_package"`
		} `json:"metadata"`
	} `json:"venvs"`
}

// parsePipxList는 pipx list --json을 파싱한다(exec와 분리 — 테스트 대상).
func parsePipxList(data []byte) []report.NameVersion {
	var raw pipxRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return []report.NameVersion{}
	}
	out := make([]report.NameVersion, 0, len(raw.Venvs))
	for name, v := range raw.Venvs {
		out = append(out, report.NameVersion{
			Name:    name,
			Version: report.PtrOrNil(v.Metadata.MainPackage.PackageVersion),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

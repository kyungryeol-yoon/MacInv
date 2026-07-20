package source

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// brewSource는 Homebrew formulae/casks를 스캔한다.
type brewSource struct {
	run *iexec.Runner
}

func (s *brewSource) Name() string   { return NameBrew }
func (s *brewSource) Available() bool { _, err := exec.LookPath("brew"); return err == nil }

func (s *brewSource) Scan(ctx context.Context) (any, error) {
	prefix, _ := s.run.Run(ctx, iexec.Opt{TimeoutSec: 30}, "brew", "--prefix")
	brew := &report.Brew{
		Prefix:   report.PtrOrNil(strings.TrimSpace(prefix)),
		Formulae: []report.Formula{},
		Casks:    []report.Cask{},
	}

	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 300}, "brew", "info", "--json=v2", "--installed")
	if err != nil {
		return brew, nil // prefix만 채운 채로 반환(프로토타입과 동일)
	}
	f, c, perr := parseBrewInfo([]byte(out))
	if perr != nil {
		s.run.Rec.Add("brew info: JSON 파싱 실패")
		return brew, nil
	}
	brew.Formulae = f
	brew.Casks = c
	return brew, nil
}

// brewInfoRaw는 brew info --json=v2 출력의 필요한 필드만 담는다.
type brewInfoRaw struct {
	Formulae []struct {
		Name         string   `json:"name"`
		Dependencies []string `json:"dependencies"`
		Installed    []struct {
			Version             string `json:"version"`
			InstalledOnRequest  bool   `json:"installed_on_request"`
			InstalledAsDependency bool `json:"installed_as_dependency"`
		} `json:"installed"`
	} `json:"formulae"`
	Casks []struct {
		Token     string `json:"token"`
		Installed string `json:"installed"`
		Artifacts []json.RawMessage `json:"artifacts"`
	} `json:"casks"`
}

// parseBrewInfo는 brew info --json=v2 JSON을 파싱한다(exec와 분리 — 테스트 대상).
func parseBrewInfo(data []byte) ([]report.Formula, []report.Cask, error) {
	var raw brewInfoRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	formulae := make([]report.Formula, 0, len(raw.Formulae))
	for _, f := range raw.Formulae {
		var inst struct {
			Version               string
			InstalledOnRequest    bool
			InstalledAsDependency bool
		}
		if len(f.Installed) > 0 {
			inst.Version = f.Installed[0].Version
			inst.InstalledOnRequest = f.Installed[0].InstalledOnRequest
			inst.InstalledAsDependency = f.Installed[0].InstalledAsDependency
		}
		deps := f.Dependencies
		if deps == nil {
			deps = []string{}
		}
		formulae = append(formulae, report.Formula{
			Name:                  f.Name,
			Version:               report.PtrOrNil(inst.Version),
			InstalledOnRequest:    inst.InstalledOnRequest,
			InstalledAsDependency: inst.InstalledAsDependency,
			Dependencies:          deps,
			Outdated:              false,
		})
	}
	casks := make([]report.Cask, 0, len(raw.Casks))
	for _, c := range raw.Casks {
		casks = append(casks, report.Cask{
			Name:     c.Token,
			Version:  report.PtrOrNil(c.Installed),
			AppNames: appNamesFromArtifacts(c.Artifacts),
			Outdated: false,
		})
	}
	return formulae, casks, nil
}

// appNamesFromArtifacts는 artifacts에서 .app로 끝나는 문자열 항목만 추린다.
func appNamesFromArtifacts(arts []json.RawMessage) []string {
	names := []string{}
	for _, a := range arts {
		var str string
		if err := json.Unmarshal(a, &str); err == nil && strings.HasSuffix(str, ".app") {
			names = append(names, str)
		}
	}
	return names
}

// EnrichBrew는 brew outdated --json=v2 결과로 outdated/latest_version을 채운다.
func EnrichBrew(ctx context.Context, run *iexec.Runner, brew *report.Brew) {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 300}, "brew", "outdated", "--json=v2")
	if err != nil {
		return
	}
	applyBrewOutdated([]byte(out), brew)
}

type brewOutdatedRaw struct {
	Formulae []struct {
		Name           string `json:"name"`
		CurrentVersion string `json:"current_version"`
	} `json:"formulae"`
	Casks []struct {
		Name           string `json:"name"`
		CurrentVersion string `json:"current_version"`
	} `json:"casks"`
}

// applyBrewOutdated는 outdated JSON을 파싱해 brew에 반영한다(exec와 분리 — 테스트 대상).
func applyBrewOutdated(data []byte, brew *report.Brew) {
	var raw brewOutdatedRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	fIdx := map[string]int{}
	for i := range brew.Formulae {
		fIdx[brew.Formulae[i].Name] = i
	}
	for _, o := range raw.Formulae {
		if i, ok := fIdx[o.Name]; ok {
			brew.Formulae[i].Outdated = true
			brew.Formulae[i].LatestVersion = report.PtrOrNil(o.CurrentVersion)
		}
	}
	cIdx := map[string]int{}
	for i := range brew.Casks {
		cIdx[brew.Casks[i].Name] = i
	}
	for _, o := range raw.Casks {
		if i, ok := cIdx[o.Name]; ok {
			brew.Casks[i].Outdated = true
			brew.Casks[i].LatestVersion = report.PtrOrNil(o.CurrentVersion)
		}
	}
}

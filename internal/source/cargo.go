package source

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// cargoSource는 cargo install --list 결과를 스캔한다.
type cargoSource struct {
	run *iexec.Runner
}

func (s *cargoSource) Name() string   { return NameCargo }
func (s *cargoSource) Available() bool { _, err := exec.LookPath("cargo"); return err == nil }

func (s *cargoSource) Scan(ctx context.Context) (any, error) {
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "cargo", "install", "--list")
	if err != nil {
		return []report.NameVersion{}, nil
	}
	return parseCargoList(out), nil
}

// cargo install --list 형식:
//
//	ripgrep v14.1.0:
//	    rg
//
// 들여쓰기 없는 줄이 패키지, 들여쓰기 줄은 바이너리 이름.
var cargoLine = regexp.MustCompile(`^(\S+)\s+v?([\w.\-+]+)`)

// parseCargoList는 cargo install --list 출력을 파싱한다(exec와 분리 — 테스트 대상).
func parseCargoList(out string) []report.NameVersion {
	pkgs := []report.NameVersion{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		m := cargoLine.FindStringSubmatch(strings.TrimSuffix(strings.TrimRight(line, "\r"), ":"))
		if m != nil {
			pkgs = append(pkgs, report.NameVersion{
				Name:    m[1],
				Version: report.PtrOrNil(m[2]),
			})
		}
	}
	return pkgs
}

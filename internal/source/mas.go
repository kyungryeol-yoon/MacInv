package source

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

// masSource는 App Store(mas) 설치 앱과 업데이트 대기 목록을 스캔한다.
type masSource struct {
	run *iexec.Runner
}

func (s *masSource) Name() string    { return NameMas }
func (s *masSource) Available() bool { _, err := exec.LookPath("mas"); return err == nil }

func (s *masSource) Scan(ctx context.Context) (any, error) {
	list, _ := s.run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "mas", "list")
	outdated, _ := s.run.Run(ctx, iexec.Opt{TimeoutSec: 120}, "mas", "outdated")
	return parseMas(list, outdated), nil
}

// mas list 형식:   497799835 Xcode (26.4.1)
var masListLine = regexp.MustCompile(`^(\d+)\s+(.+?)\s+\(([^)]*)\)\s*$`)

// mas outdated 형식:  497799835 Xcode (26.4.0 -> 26.4.1)
var masOutdatedLine = regexp.MustCompile(`^(\d+)\s+(.+?)\s+\(([^)]*)\s*->\s*([^)]*)\)\s*$`)

// parseMas는 mas list/outdated 출력을 파싱한다(exec와 분리 — 테스트 대상).
func parseMas(list, outdated string) *report.Mas {
	mas := &report.Mas{Apps: []report.MasApp{}, Outdated: []report.MasOutdated{}}
	for _, line := range strings.Split(list, "\n") {
		if m := masListLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			mas.Apps = append(mas.Apps, report.MasApp{ID: m[1], Name: m[2], Version: m[3]})
		}
	}
	for _, line := range strings.Split(outdated, "\n") {
		if m := masOutdatedLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			mas.Outdated = append(mas.Outdated, report.MasOutdated{
				ID: m[1], Name: m[2],
				Version:       strings.TrimSpace(m[3]),
				LatestVersion: strings.TrimSpace(m[4]),
			})
		}
	}
	return mas
}

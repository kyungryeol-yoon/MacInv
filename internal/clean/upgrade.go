package clean

import (
	"context"
	"regexp"
	"strings"
	"sync"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// KindMas는 App Store(mas) 앱 타깃이다(업데이트 전용).
const KindMas = "mas"

// upgradeTimeoutSec는 업그레이드/설치 명령 타임아웃(30분)이다.
// 대형 cask 다운로드 대비. 실행은 비동기+단일 큐라 UI에 영향 없고, 삭제·휴지통은 기본(300초) 유지.
const upgradeTimeoutSec = 1800

// BuildUpgradePlan은 name을 최신 버전으로 올리는 계획을 만든다(읽기 전용).
// 소스별 네이티브 업그레이드 명령만 담고 삭제/휴지통은 없다. Execute·RenderPlanText를 삭제와 공유한다.
// 4개 소스 프로브(각각 외부 명령 1회)는 서로 독립이라 병렬로 조회한다 — TUI 모달 대기시간 단축.
func BuildUpgradePlan(ctx context.Context, run *iexec.Runner, name string) *Plan {
	plan := &Plan{Query: name, Verb: "업데이트", Icon: "⬆️"}

	var (
		wg                       sync.WaitGroup
		formulaOK, caskOK, npmOK bool
		masID                    string
		masOK                    bool
	)
	wg.Add(4)
	go func() { defer wg.Done(); formulaOK = brewInstalled(ctx, run, name, false) }()
	go func() { defer wg.Done(); caskOK = brewInstalled(ctx, run, name, true) }()
	go func() { defer wg.Done(); npmOK = npmGlobalInstalled(ctx, run, name) }()
	go func() { defer wg.Done(); masID, masOK = masAppID(ctx, run, name) }()
	wg.Wait()

	if formulaOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindBrewFormula, Name: name,
			Argv:          [][]string{{"brew", "upgrade", name}},
			CmdTimeoutSec: upgradeTimeoutSec,
		})
	}
	if caskOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindBrewCask, Name: name,
			Argv:          [][]string{{"brew", "upgrade", "--cask", name}},
			CmdTimeoutSec: upgradeTimeoutSec,
		})
	}
	if npmOK {
		plan.Targets = append(plan.Targets, npmUpgradeTarget(name))
	}
	if masOK {
		plan.Targets = append(plan.Targets, Target{
			Kind: KindMas, Name: name,
			Argv:          [][]string{{"mas", "upgrade", masID}},
			CmdTimeoutSec: upgradeTimeoutSec,
		})
	}
	return plan
}

// npmUpgradeTarget은 npm 전역 패키지 업그레이드 타깃을 만든다(순수 함수 — 테스트 대상).
// npm 셀프 업데이트는 실행 중인 npm이 자기 모듈 파일을 지우다 MODULE_NOT_FOUND로 죽을 수 있어
// npx로 최신 npm을 캐시에 받아 그걸로 설치한다(자기 파일을 안 건드림).
func npmUpgradeTarget(name string) Target {
	t := Target{Kind: KindNpm, Name: name, CmdTimeoutSec: upgradeTimeoutSec}
	if name == "npm" {
		t.Argv = [][]string{{"npx", "-y", "npm@latest", "install", "-g", "npm@latest"}}
		t.Warnings = []string{"npm 자체 업데이트는 안전을 위해 npx 경유로 실행합니다(셀프 업데이트 실패 방지)."}
	} else {
		t.Argv = [][]string{{"npm", "install", "-g", name + "@latest"}}
	}
	return t
}

// UpgradePlanFromOutdated는 outdated 목록에서 곧장 일괄 업데이트 계획을 만든다(순수 함수 — 테스트 대상).
// 각 행의 소스를 이미 알므로 설치 여부 프로브가 필요 없다(update --all 경로, 빠름).
func UpgradePlanFromOutdated(rows []report.OutdatedRow) *Plan {
	plan := &Plan{Query: "전체 업데이트", Verb: "업데이트", Icon: "⬆️"}
	for _, r := range rows {
		switch r.Source {
		case "brew":
			plan.Targets = append(plan.Targets, Target{
				Kind: KindBrewFormula, Name: r.Name,
				Argv:          [][]string{{"brew", "upgrade", r.Name}},
				CmdTimeoutSec: upgradeTimeoutSec,
			})
		case "cask":
			plan.Targets = append(plan.Targets, Target{
				Kind: KindBrewCask, Name: r.Name,
				Argv:          [][]string{{"brew", "upgrade", "--cask", r.Name}},
				CmdTimeoutSec: upgradeTimeoutSec,
			})
		case "npm":
			plan.Targets = append(plan.Targets, npmUpgradeTarget(r.Name))
		case "mas":
			if r.ID != "" {
				plan.Targets = append(plan.Targets, Target{
					Kind: KindMas, Name: r.Name,
					Argv:          [][]string{{"mas", "upgrade", r.ID}},
					CmdTimeoutSec: upgradeTimeoutSec,
				})
			}
		}
	}
	return plan
}

// BuildCombinedUpgradePlan은 여러 대상의 업데이트 계획을 하나로 합친다(TUI 멀티선택용).
func BuildCombinedUpgradePlan(ctx context.Context, run *iexec.Runner, names []string) *Plan {
	combined := &Plan{Query: strings.Join(names, ", "), Verb: "업데이트", Icon: "⬆️"}
	for _, n := range names {
		p := BuildUpgradePlan(ctx, run, n)
		combined.Targets = append(combined.Targets, p.Targets...)
	}
	return combined
}

var masListLine = regexp.MustCompile(`^(\d+)\s+(.+?)\s+\(`)

// masAppID는 mas list에서 앱 이름으로 App Store ID를 찾는다.
func masAppID(ctx context.Context, run *iexec.Runner, name string) (string, bool) {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "mas", "list")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(out, "\n") {
		if m := masListLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if strings.EqualFold(strings.TrimSpace(m[2]), name) {
				return m[1], true
			}
		}
	}
	return "", false
}

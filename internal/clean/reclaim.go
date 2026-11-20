package clean

import (
	"context"
	"strconv"
	"strings"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
	"github.com/kyungryeol-yoon/MacInv/internal/paths"
	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

// 회수 예상 = "이번 삭제로 실제로 비는 용량".
//
// 앱 목록의 용량(App.DataSizeKB)은 paths.SizePaths — 측정용 **넓은** 규칙이라
// 벤더 공유 폴더까지 포함한다. 반면 삭제 계획은 공유 여부를 따로 판정해 그런 경로를 보존한다.
// 그래서 "Chrome 9.2 GB"를 보고 지웠는데 1 GB만 비는 상황이 가능하다.
// 이 파일은 **계획에 실제로 담긴 경로만** 재서 그 간극을 UI에 드러낸다.

// MeasureTrashSizes는 계획에 담긴 휴지통 경로의 용량을 재서 PathMatch.SizeKB에 채운다(읽기 전용).
// 중복 계산을 피하려고 부모/자식이 겹치는 경로는 부모 하나로 합쳐 du에 넘기고(자식은 0),
// 프로세스 스폰을 줄이려고 du -sk 한 번에 전부 넘긴다.
func MeasureTrashSizes(ctx context.Context, run *iexec.Runner, plan *Plan) {
	var all []PathMatch
	for _, t := range plan.Targets {
		all = append(all, t.TrashPaths...)
	}
	measured := paths.DedupContained(all)
	if len(measured) == 0 {
		return
	}
	args := make([]string, 0, len(measured)+1)
	args = append(args, "-sk")
	for _, m := range measured {
		args = append(args, m.Path)
	}
	// 수십 GB 캐시 트리는 walk가 오래 걸린다(scan의 dataKB와 같은 이유).
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 300}, "du", args...)
	if err != nil {
		return
	}
	sizes := parseDuSizes(out)
	// 부모로 합쳐진 경로만 값이 있고, 그 하위 경로는 0으로 남는다(합계가 두 번 세지 않게).
	keep := map[string]bool{}
	for _, m := range measured {
		keep[m.Path] = true
	}
	for ti := range plan.Targets {
		for pi := range plan.Targets[ti].TrashPaths {
			p := &plan.Targets[ti].TrashPaths[pi]
			if keep[p.Path] {
				p.SizeKB = sizes[p.Path]
				keep[p.Path] = false // 같은 경로가 여러 타깃에 있어도 한 번만 센다
			} else {
				p.SizeKB = 0
			}
		}
	}
}

// parseDuSizes는 `du -sk a b c` 출력을 경로→KB 맵으로 파싱한다(순수 함수 — 테스트 대상).
// 경로에 공백이 있을 수 있어 첫 공백 구간까지만 숫자로 보고 나머지를 통째로 경로로 쓴다.
func parseDuSizes(out string) map[string]int {
	sizes := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		i := strings.IndexAny(line, " \t")
		if i <= 0 {
			continue
		}
		n, err := strconv.Atoi(line[:i])
		if err != nil {
			continue
		}
		path := strings.TrimLeft(line[i:], " \t")
		if path == "" {
			continue
		}
		sizes[path] = n
	}
	return sizes
}

// ReclaimKB는 실제로 삭제될 경로의 용량 합계다(순수 함수 — 테스트 대상).
// enabled는 [확신낮음] 토글 상태다(nil이면 확신낮음 전부 제외 — --apply 기본값과 같다).
// ok=false면 측정값이 없어 표시할 게 없다는 뜻이다.
func ReclaimKB(p *Plan, enabled map[string]bool) (kb int, ok bool) {
	if p.ExtraReclaimKB > 0 {
		kb += p.ExtraReclaimKB
		ok = true
	}
	for _, t := range p.Targets {
		for _, pm := range t.TrashPaths {
			if pm.Confidence == ConfLow && !enabled[pm.Path] {
				continue
			}
			if pm.SizeKB > 0 {
				kb += pm.SizeKB
				ok = true
			}
		}
	}
	return kb, ok
}

// ReclaimLine은 계획 하단에 붙는 "회수 예상" 요약 줄이다(순수 함수 — 테스트 대상).
// 표시 용량(앱 목록)이 아니라 **이 계획에 담긴 경로만** 합산한 값이라는 점이 핵심이다.
// 확신낮음 경로가 더 있으면 그걸 다 포함했을 때의 값도 괄호로 같이 보여준다.
func ReclaimLine(p *Plan, enabled map[string]bool) string {
	kb, ok := ReclaimKB(p, enabled)
	if !ok {
		return ""
	}
	line := "💾 이번 삭제로 회수 예상: " + report.HumanSize(&kb)
	full, fullOK := ReclaimKB(p, allLowEnabled(p))
	if fullOK && full > kb {
		line += "  ([확신낮음] 모두 포함 시 " + report.HumanSize(&full) + ")"
	}
	return line
}

// allLowEnabled는 계획의 모든 확신낮음 경로를 켠 토글 맵이다(상한값 계산용).
func allLowEnabled(p *Plan) map[string]bool {
	on := map[string]bool{}
	for _, t := range p.Targets {
		for _, pm := range t.TrashPaths {
			if pm.Confidence == ConfLow {
				on[pm.Path] = true
			}
		}
	}
	return on
}

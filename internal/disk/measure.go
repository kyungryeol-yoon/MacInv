package disk

import (
	"context"
	"sort"

	"golang.org/x/sync/errgroup"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
)

// duTimeoutSec는 위치 하나의 용량 측정 타임아웃이다.
// 개별로 거는 이유: 26GB짜리 OrbStack 이미지 하나가 느려도 나머지 결과는 보여줘야 한다.
const duTimeoutSec = 120

// measureWorkers는 동시에 도는 du 개수다. 디스크 I/O 경합을 피해 낮게 잡는다.
const measureWorkers = 6

// Entry는 측정된 위치 하나다(JSON 출력 스키마).
type Entry struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Tool   string   `json:"tool"`
	Tier   string   `json:"tier"`
	Paths  []string `json:"paths"`
	SizeKB int      `json:"size_kb"`
	// Orphan은 캐시는 있는데 그 도구가 인벤토리에 없다는 뜻이다(지울 이유가 가장 확실한 항목).
	Orphan bool `json:"orphan"`
	// PathSizes는 경로별 용량이다. 합계만 두면 안 되는 이유가 둘 있다:
	// ① 휴지통 계획을 만들 때 경로마다 제 용량을 실어야 회수 예상이 안 부풀려진다,
	// ② 와일드카드 위치의 앱별 내역(Breakdown)을 낼 수 없다.
	PathSizes map[string]int `json:"path_sizes,omitempty"`
	// Breakdown은 GroupBy가 있는 위치의 내역이다(용량순). 없으면 nil.
	Breakdown []Usage `json:"breakdown,omitempty"`
	// BlockedByRunning은 소유 앱이 **실행 중**이라 정리하면 안 되는 경로다(경로 → 앱 이름).
	// Chromium은 캐시 핸들을 연 채로 돌아서, 켜져 있을 때 지우면 앱이 오작동한다.
	BlockedByRunning map[string]string `json:"blocked_by_running,omitempty"`
	Cleanup          [][]string        `json:"cleanup,omitempty"`
	Trash            bool              `json:"trash,omitempty"`
	Note             string            `json:"note,omitempty"`
	// GroupBy는 Location에서 그대로 온다. 계획 단계에서도 "이 위치가 실행 중 앱 보호
	// 대상인가"를 알아야 해서 Entry에 남긴다(app-support-dir = Electron 캐시).
	GroupBy string `json:"group_by,omitempty"`
}

// Result는 disk 커맨드의 전체 결과다.
type Result struct {
	Entries []Entry `json:"entries"`
	TotalKB int     `json:"total_kb"`
	// ReclaimableKB는 기본 정리 대상(safe+artifact)의 합계다.
	ReclaimableKB int `json:"reclaimable_kb"`
	// ExpensiveKB는 --include-expensive를 줘야 정리되는 몫이다.
	ExpensiveKB int `json:"expensive_kb"`
	// RunningCheckFailed는 실행 중인 앱 조회가 실패했다는 뜻이다.
	// 이때 Electron 캐시 보호가 동작하지 않으므로 **반드시 사용자에게 알려야 한다** —
	// 보호가 도는 줄 알고 --apply를 누르면 실행 중인 앱의 캐시가 그대로 지워진다.
	RunningCheckFailed bool            `json:"running_check_failed,omitempty"`
	Snapshots          *SnapshotReport `json:"snapshots,omitempty"`
	Deep               []DeepFinding   `json:"deep,omitempty"`
}

// Scan은 알려진 위치들을 병렬로 측정해 용량순으로 돌려준다(읽기 전용).
// installedTools가 nil이 아니면 고아 캐시 여부를 함께 판정한다.
func Scan(ctx context.Context, run *iexec.Runner, home string, installedTools map[string]bool) *Result {
	locs := Locations()
	byID := ResolvePaths(home, locs)
	// 실행 중인 앱 조회는 ps 1회 — 경로마다 부르지 않는다.
	running, runningOK := RunningApps(ctx, run)

	entries := make([]Entry, 0, len(locs))
	for _, l := range locs {
		ps := byID[l.ID]
		if len(ps) == 0 {
			continue // 이 맥에 없는 위치는 조용히 스킵(미설치는 에러가 아니다)
		}
		entries = append(entries, Entry{
			ID: l.ID, Name: l.Name, Tool: l.Tool, Tier: l.Tier,
			Paths: ps, Cleanup: l.Cleanup, Trash: l.Trash, Note: l.Note,
			Orphan:  installedTools != nil && IsOrphanCache(l, installedTools),
			GroupBy: l.GroupBy,
		})
	}

	// 위치마다 du -sk 한 번(경로는 한 번에 넘긴다). errgroup으로 병렬, 개별 타임아웃.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(measureWorkers)
	for i := range entries {
		g.Go(func() error {
			sizes := duSizes(gctx, run, entries[i].Paths)
			entries[i].PathSizes = sizes
			for _, kb := range sizes {
				entries[i].SizeKB += kb
			}
			entries[i].Breakdown = GroupUsage(home, entries[i].GroupBy, sizes)
			entries[i].BlockedByRunning = blockedByRunning(home, entries[i].GroupBy, entries[i].Paths, running)
			return nil // 개별 실패는 0KB로 두고 전체를 멈추지 않는다
		})
	}
	_ = g.Wait()

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].SizeKB > entries[j].SizeKB })

	res := &Result{Entries: entries, RunningCheckFailed: !runningOK}
	for _, e := range entries {
		res.TotalKB += e.SizeKB
		switch {
		case Cleanable(e.Tier, false):
			res.ReclaimableKB += e.SizeKB
		case e.Tier == TierExpensive:
			res.ExpensiveKB += e.SizeKB
		}
	}
	return res
}

// duSizes는 경로별 KB를 구한다(du -sk 1회, 프로세스 스폰 절약).
func duSizes(ctx context.Context, run *iexec.Runner, ps []string) map[string]int {
	if len(ps) == 0 {
		return nil
	}
	args := append([]string{"-sk"}, ps...)
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: duTimeoutSec}, "du", args...)
	if err != nil {
		return nil
	}
	return parseDuLines(out)
}

// SumDuKB는 `du -sk a b c` 출력의 KB 합계를 낸다(순수 함수 — 테스트 대상).
// 권한 오류로 일부 경로가 빠져도 나머지는 그대로 합산한다.
func SumDuKB(out string) int {
	total := 0
	for _, kb := range parseDuLines(out) {
		total += kb
	}
	return total
}

package disk

import (
	"fmt"
	"io"
	"strings"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// PrintSummary는 disk 결과를 한국어 요약으로 출력한다.
func PrintSummary(w io.Writer, res *Result, includeExpensive bool) {
	line := strings.Repeat("─", 74)
	fmt.Fprintf(w, "\n%s\n 💽 디스크 사용처 (알려진 캐시·데이터 위치)\n%s\n", line, line)

	if len(res.Entries) == 0 {
		fmt.Fprintf(w, "  알려진 위치를 하나도 찾지 못했습니다.\n\n")
		return
	}

	for _, e := range res.Entries {
		orphan := ""
		if e.Orphan {
			orphan = "  ⚠️ [고아 캐시]"
		}
		fmt.Fprintf(w, "  %9s  %-16s %-34s %s%s\n",
			report.HumanSize(&e.SizeKB), TierLabel(e.Tier), trim(e.Name, 34), trim(e.Tool, 12), orphan)
	}

	fmt.Fprintf(w, "\n  %-28s %s\n", "합계", report.HumanSize(&res.TotalKB))
	fmt.Fprintf(w, "  %-28s %s\n", "정리 시 회수 가능(안전+산출물)", report.HumanSize(&res.ReclaimableKB))
	if res.ExpensiveKB > 0 {
		hint := " (--include-expensive 필요)"
		if includeExpensive {
			hint = " (포함됨)"
		}
		fmt.Fprintf(w, "  %-28s %s%s\n", "재다운로드 비싼 항목", report.HumanSize(&res.ExpensiveKB), hint)
	}

	printOrphans(w, line, res)
	printDeep(w, line, res.Deep)
	printSnapshots(w, line, res.Snapshots)
}

// printOrphans는 고아 캐시를 따로 모아 보여준다.
// macinv만 할 수 있는 판단이라(인벤토리 대조) 목록에 묻히지 않게 별도 섹션으로 낸다.
func printOrphans(w io.Writer, line string, res *Result) {
	var orphans []Entry
	total := 0
	for _, e := range res.Entries {
		if e.Orphan {
			orphans = append(orphans, e)
			total += e.SizeKB
		}
	}
	if len(orphans) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n ⚠️  고아 캐시 — 도구는 없는데 캐시만 남음 (%s)\n%s\n", line, report.HumanSize(&total), line)
	for _, e := range orphans {
		fmt.Fprintf(w, "  %9s  %-34s '%s'가 인벤토리에 없습니다\n", report.HumanSize(&e.SizeKB), trim(e.Name, 34), e.Tool)
		for _, p := range e.Paths {
			fmt.Fprintf(w, "             %s\n", p)
		}
	}
}

func printDeep(w io.Writer, line string, deep []DeepFinding) {
	if len(deep) == 0 {
		return
	}
	total := 0
	for _, d := range deep {
		total += d.SizeKB
	}
	fmt.Fprintf(w, "\n%s\n 🔎 오래된 node_modules (%d개 · %s · 6개월 이상 미수정)\n%s\n",
		line, len(deep), report.HumanSize(&total), line)
	for _, d := range deep {
		fmt.Fprintf(w, "  %9s  %s  (최종 수정 %s · %d일 전)\n",
			report.HumanSize(&d.SizeKB), d.Path, d.ModTime, d.AgeDays)
	}
	fmt.Fprintf(w, "\n  프로젝트 디렉토리에서 다시 설치하면 복구됩니다(npm install).\n")
}

// printSnapshots는 "지워도 용량이 안 늘어나는 이유"를 안내한다. 스냅샷을 삭제하지는 않는다.
func printSnapshots(w io.Writer, line string, snap *SnapshotReport) {
	if snap == nil {
		return
	}
	fmt.Fprintf(w, "\n%s\n 📸 APFS 로컬 스냅샷\n%s\n", line, line)
	if len(snap.Names) == 0 {
		fmt.Fprintf(w, "  없음 — 지운 용량은 바로 반영됩니다.\n")
		return
	}
	fmt.Fprintf(w, "  %d개의 Time Machine 로컬 스냅샷이 있습니다:\n", len(snap.Names))
	for _, n := range snap.Names {
		fmt.Fprintf(w, "    • %s\n", n)
	}
	fmt.Fprintf(w, "\n  ⚠️  파일을 지워도 스냅샷이 블록을 붙들고 있어 여유 공간이 바로 늘지 않을 수 있습니다.\n")
	fmt.Fprintf(w, "  (df 기준 사용 %s / 여유 %s)\n",
		report.HumanSize(&snap.UsedKB), report.HumanSize(&snap.AvailKB))
	fmt.Fprintf(w, "  macinv는 스냅샷을 삭제하지 않습니다 — 백업 복원 지점이라 직접 판단하세요:\n")
	fmt.Fprintf(w, "    tmutil deletelocalsnapshots <날짜>   (보통은 며칠 뒤 자동으로 정리됩니다)\n")
}

func trim(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

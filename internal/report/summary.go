package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// HumanSize는 KB를 사람이 읽기 쉬운 단위로 바꾼다.
func HumanSize(kb *int) string {
	if kb == nil {
		return "-"
	}
	v := *kb
	switch {
	case v >= 1024*1024:
		return fmt.Sprintf("%.1f GB", float64(v)/1024/1024)
	case v >= 1024:
		return fmt.Sprintf("%.0f MB", float64(v)/1024)
	default:
		return fmt.Sprintf("%d KB", v)
	}
}

// PrintSummary는 프로토타입과 동일한 한국어 요약을 w에 출력한다.
func PrintSummary(w io.Writer, r *Report) {
	s := r.Sections
	line := strings.Repeat("─", 62)
	fmt.Fprintf(w, "\n%s\n 📦 macOS 설치 인벤토리 요약  (%s)\n%s\n", line, safe19(r.GeneratedAt), line)

	fmt.Fprintf(w, "  GUI 앱               : %d개\n", len(s.Apps))

	if s.Brew != nil {
		leaves := 0
		for _, f := range s.Brew.Formulae {
			if f.InstalledOnRequest {
				leaves++
			}
		}
		deps := len(s.Brew.Formulae) - leaves
		fmt.Fprintf(w, "  brew formulae        : %d개 (직접 설치 %d / 의존성 %d)\n", len(s.Brew.Formulae), leaves, deps)
		fmt.Fprintf(w, "  brew casks           : %d개\n", len(s.Brew.Casks))
	} else {
		fmt.Fprintf(w, "  brew                 : 미설치\n")
	}

	if s.Npm != nil {
		fmt.Fprintf(w, "  npm 글로벌           : %d개\n", len(s.Npm.Packages))
	} else {
		fmt.Fprintf(w, "  npm                  : 미설치\n")
	}

	for _, kv := range []struct {
		label string
		n     int
		show  bool
	}{
		{"pipx", len(s.Pipx), s.Pipx != nil},
		{"cargo", len(s.Cargo), s.Cargo != nil},
		{"~/go/bin", len(s.GoBin), s.GoBin != nil},
	} {
		if kv.show {
			fmt.Fprintf(w, "  %-20s : %d개\n", kv.label, kv.n)
		}
	}

	if s.Mas != nil {
		fmt.Fprintf(w, "  App Store (mas)      : %d개\n", len(s.Mas.Apps))
	}

	fmt.Fprintf(w, "  LaunchAgents/Daemons : %d개\n", len(s.Launchd))

	// 업데이트 대기 목록
	rows := OutdatedRows(r)
	if len(rows) > 0 {
		fmt.Fprintf(w, "\n%s\n ⬆️  업데이트 대기 중 (%d개)\n%s\n", line, len(rows), line)
		for _, row := range rows {
			fmt.Fprintf(w, "  [%-4s] %-30s %-15s → %s\n", row.Source, row.Name, row.Current, row.Latest)
		}
	} else if !r.OutdatedSkipped {
		fmt.Fprintf(w, "\n  ✅ 모든 패키지가 최신 상태입니다.\n")
	}

	// 용량 상위 앱 — 번들만이 아니라 데이터/캐시를 합친 실제 디스크 점유 기준.
	printSizeSummary(w, line, s.Apps)

	// 개발 런타임 (설치된 언어 버전)
	printRuntimeSummary(w, line, r.Runtimes, r.VersionManagers)

	if len(r.Errors) > 0 {
		fmt.Fprintf(w, "\n  ⚠️  경고 %d건 (상세는 JSON errors 필드 참고)\n", len(r.Errors))
	}
	fmt.Fprintln(w)
}

// TopBySize는 앱을 실제 디스크 점유(번들+데이터) 내림차순으로 정렬해 상위 n개를 돌려준다.
// 용량을 못 잰 앱(nil)과 0바이트는 제외한다. 순수 함수 — 테스트 대상.
func TopBySize(apps []App, n int) []App {
	sized := make([]App, 0, len(apps))
	for _, a := range apps {
		if t := a.TotalSizeKB(); t != nil && *t > 0 {
			sized = append(sized, a)
		}
	}
	sort.SliceStable(sized, func(i, j int) bool {
		return *sized[i].TotalSizeKB() > *sized[j].TotalSizeKB()
	})
	if n > 0 && len(sized) > n {
		sized = sized[:n]
	}
	return sized
}

// printSizeSummary는 "용량 큰 앱 Top 10"을 번들/데이터 내역과 함께 출력한다.
func printSizeSummary(w io.Writer, line string, apps []App) {
	top := TopBySize(apps, 10)
	if len(top) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n 💾 용량 큰 앱 Top 10  (앱 번들 + 데이터/캐시)\n%s\n", line, line)
	for _, a := range top {
		fmt.Fprintf(w, "  %9s  %-28s %s\n",
			HumanSize(a.TotalSizeKB()), trimName(a.Name, 28), sizeBreakdown(a))
	}
	fmt.Fprintf(w, "\n  데이터가 큰 앱은 `macinv clean <이름>`으로 지울 항목을 확인할 수 있습니다.\n")
}

// sizeBreakdown은 "앱 300 MB + 데이터 67.3 GB" 꼴의 내역 문자열을 만든다.
// 데이터가 없거나 0이면 내역을 생략한다(합계만으로 충분).
func sizeBreakdown(a App) string {
	if a.DataSizeKB == nil || *a.DataSizeKB == 0 {
		return ""
	}
	return fmt.Sprintf("앱 %s + 데이터 %s", HumanSize(a.SizeKB), HumanSize(a.DataSizeKB))
}

// trimName은 요약 표의 이름 칸을 넘치지 않게 자른다.
func trimName(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// printRuntimeSummary는 설치된 개발 런타임(언어) 버전을 요약에 출력한다.
func printRuntimeSummary(w io.Writer, line string, runtimes []Runtime, managers []VersionManager) {
	installed := make([]Runtime, 0, len(runtimes))
	for _, rt := range runtimes {
		if rt.Installed {
			installed = append(installed, rt)
		}
	}
	if len(installed) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n 🧰 개발 런타임 (설치된 언어)\n%s\n", line, line)
	for _, rt := range installed {
		ver := rt.Version
		if ver == "" {
			ver = "?"
		}
		mgr := ""
		conflict := ""
		if len(rt.Origins) > 0 {
			mgr = rt.Origins[0].Manager
		}
		if rt.Conflict {
			conflict = "  ⚠️ 여러 경로"
		}
		fmt.Fprintf(w, "  %-8s %-10s [%s]%s\n", rt.Name, ver, mgr, conflict)
	}
	if len(managers) > 0 {
		var names []string
		for _, m := range managers {
			names = append(names, m.Name)
		}
		fmt.Fprintf(w, "  버전 매니저: %s (자세히: macinv runtimes)\n", strings.Join(names, ", "))
	}
}

func deref(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

// safe19는 ISO 타임스탬프의 앞 19자(YYYY-MM-DD HH:MM:SS 부분)만 취한다.
func safe19(s string) string {
	if len(s) >= 19 {
		return s[:19]
	}
	return s
}

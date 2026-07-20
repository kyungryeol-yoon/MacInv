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

	// 용량 상위 앱
	sized := make([]App, 0, len(s.Apps))
	for _, a := range s.Apps {
		if a.SizeKB != nil && *a.SizeKB > 0 {
			sized = append(sized, a)
		}
	}
	if len(sized) > 0 {
		sort.Slice(sized, func(i, j int) bool { return *sized[i].SizeKB > *sized[j].SizeKB })
		fmt.Fprintf(w, "\n%s\n 💾 용량 큰 앱 Top 10\n%s\n", line, line)
		for i, a := range sized {
			if i >= 10 {
				break
			}
			fmt.Fprintf(w, "  %9s  %s\n", HumanSize(a.SizeKB), a.Name)
		}
	}

	// 개발 런타임 (설치된 언어 버전)
	printRuntimeSummary(w, line, r.Runtimes, r.VersionManagers)

	if len(r.Errors) > 0 {
		fmt.Fprintf(w, "\n  ⚠️  경고 %d건 (상세는 JSON errors 필드 참고)\n", len(r.Errors))
	}
	fmt.Fprintln(w)
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

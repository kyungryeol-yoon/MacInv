package report

import "sort"

// OutdatedRow는 업데이트 대기 중인 항목 한 개다(요약/outdated 커맨드 공용).
type OutdatedRow struct {
	Source  string `json:"source"` // brew | cask | npm | mas
	Name    string `json:"name"`
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

// OutdatedRows는 리포트에서 업데이트 대기 항목을 모아 source, name 순으로 정렬해 돌려준다.
func OutdatedRows(r *Report) []OutdatedRow {
	var rows []OutdatedRow
	s := r.Sections
	if s.Brew != nil {
		for _, f := range s.Brew.Formulae {
			if f.Outdated {
				rows = append(rows, OutdatedRow{"brew", f.Name, deref(f.Version), deref(f.LatestVersion)})
			}
		}
		for _, c := range s.Brew.Casks {
			if c.Outdated {
				rows = append(rows, OutdatedRow{"cask", c.Name, deref(c.Version), deref(c.LatestVersion)})
			}
		}
	}
	if s.Npm != nil {
		for _, p := range s.Npm.Packages {
			if p.Outdated {
				rows = append(rows, OutdatedRow{"npm", p.Name, deref(p.Version), deref(p.LatestVersion)})
			}
		}
	}
	if s.Mas != nil {
		for _, o := range s.Mas.Outdated {
			rows = append(rows, OutdatedRow{"mas", o.Name, o.Version, o.LatestVersion})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

package report

import "sort"

// OutdatedRow는 업데이트 대기 중인 항목 한 개다(요약/outdated 커맨드 공용).
type OutdatedRow struct {
	Source  string `json:"source"` // brew | cask | npm | mas
	Name    string `json:"name"`
	Current string `json:"current"`
	Latest  string `json:"latest"`
	ID      string `json:"id,omitempty"` // mas 전용: App Store 앱 ID (mas upgrade에 필요)
}

// OutdatedRows는 리포트에서 업데이트 대기 항목을 모아 source, name 순으로 정렬해 돌려준다.
func OutdatedRows(r *Report) []OutdatedRow {
	var rows []OutdatedRow
	s := r.Sections
	if s.Brew != nil {
		for _, f := range s.Brew.Formulae {
			if f.Outdated {
				rows = append(rows, OutdatedRow{Source: "brew", Name: f.Name, Current: deref(f.Version), Latest: deref(f.LatestVersion)})
			}
		}
		for _, c := range s.Brew.Casks {
			if c.Outdated {
				rows = append(rows, OutdatedRow{Source: "cask", Name: c.Name, Current: deref(c.Version), Latest: deref(c.LatestVersion)})
			}
		}
	}
	if s.Npm != nil {
		for _, p := range s.Npm.Packages {
			if p.Outdated {
				rows = append(rows, OutdatedRow{Source: "npm", Name: p.Name, Current: deref(p.Version), Latest: deref(p.LatestVersion)})
			}
		}
	}
	if s.Mas != nil {
		for _, o := range s.Mas.Outdated {
			rows = append(rows, OutdatedRow{Source: "mas", Name: o.Name, Current: o.Version, Latest: o.LatestVersion, ID: o.ID})
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

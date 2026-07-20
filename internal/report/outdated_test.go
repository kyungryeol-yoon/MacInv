package report

import "testing"

func TestOutdatedRows(t *testing.T) {
	r := &Report{Sections: Sections{
		Brew: &Brew{
			Formulae: []Formula{
				{Name: "git", Version: Ptr("2.53.0"), LatestVersion: Ptr("2.55.0"), Outdated: true},
				{Name: "hugo", Version: Ptr("0.164.0"), Outdated: false}, // 최신 → 제외
			},
			Casks: []Cask{
				{Name: "iterm2", Version: Ptr("3.5.6"), LatestVersion: Ptr("3.5.9"), Outdated: true},
			},
		},
		Npm: &Npm{Packages: []NpmPackage{
			{Name: "npm", Version: Ptr("10.9.7"), LatestVersion: Ptr("12.0.1"), Outdated: true},
		}},
		Mas: &Mas{Outdated: []MasOutdated{
			{Name: "Xcode", Version: "26.4.0", LatestVersion: "26.4.1"},
		}},
	}}

	rows := OutdatedRows(r)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (hugo 제외)", len(rows))
	}
	// source, name 순 정렬: brew < cask < mas < npm.
	wantOrder := []string{"brew", "cask", "mas", "npm"}
	for i, w := range wantOrder {
		if rows[i].Source != w {
			t.Errorf("rows[%d].Source = %s, want %s", i, rows[i].Source, w)
		}
	}
	if rows[0].Name != "git" || rows[0].Current != "2.53.0" || rows[0].Latest != "2.55.0" {
		t.Errorf("brew 행 오류: %+v", rows[0])
	}
	if rows[2].Name != "Xcode" || rows[2].Latest != "26.4.1" {
		t.Errorf("mas 행 오류: %+v", rows[2])
	}
}

func TestOutdatedRowsEmpty(t *testing.T) {
	r := &Report{Sections: Sections{Brew: &Brew{}, Npm: &Npm{}}}
	if rows := OutdatedRows(r); len(rows) != 0 {
		t.Errorf("빈 리포트인데 rows = %d", len(rows))
	}
}

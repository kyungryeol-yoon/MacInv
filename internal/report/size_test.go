package report

import "testing"

func TestTotalSizeKB(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want *int
	}{
		{"둘 다 없으면 nil", App{}, nil},
		{"번들만", App{SizeKB: Ptr(100)}, Ptr(100)},
		{"데이터만 (--no-sizes 아닌데 번들 du 실패)", App{DataSizeKB: Ptr(70)}, Ptr(70)},
		{"합계", App{SizeKB: Ptr(300), DataSizeKB: Ptr(70453708)}, Ptr(70454008)},
	}
	for _, tt := range tests {
		got := tt.app.TotalSizeKB()
		switch {
		case tt.want == nil && got != nil:
			t.Errorf("%s: TotalSizeKB = %d, want nil", tt.name, *got)
		case tt.want != nil && got == nil:
			t.Errorf("%s: TotalSizeKB = nil, want %d", tt.name, *tt.want)
		case tt.want != nil && *got != *tt.want:
			t.Errorf("%s: TotalSizeKB = %d, want %d", tt.name, *got, *tt.want)
		}
	}
}

func TestTopBySize(t *testing.T) {
	apps := []App{
		{Name: "Xcode", SizeKB: Ptr(20 * 1024 * 1024)},                               // 20GB 번들, 데이터 없음
		{Name: "Notion", SizeKB: Ptr(300 * 1024), DataSizeKB: Ptr(67 * 1024 * 1024)}, // 번들은 작지만 데이터가 67GB
		{Name: "Calculator", SizeKB: Ptr(5 * 1024)},
		{Name: "용량미측정"},              // nil → 제외
		{Name: "빈앱", SizeKB: Ptr(0)}, // 0 → 제외
	}
	got := TopBySize(apps, 10)
	if len(got) != 3 {
		t.Fatalf("길이 = %d, want 3 (nil/0은 제외)", len(got))
	}
	// 데이터를 합쳐야 Notion이 Xcode를 앞선다 — 이 기능의 핵심.
	want := []string{"Notion", "Xcode", "Calculator"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("순위 %d = %s, want %s", i, got[i].Name, name)
		}
	}
	if n := len(TopBySize(apps, 2)); n != 2 {
		t.Errorf("n=2 자르기 = %d개", n)
	}
}

func TestSizeBreakdown(t *testing.T) {
	if s := SizeBreakdown(App{SizeKB: Ptr(1024)}); s != "" {
		t.Errorf("데이터 없으면 내역 생략해야 함: %q", s)
	}
	if s := SizeBreakdown(App{SizeKB: Ptr(1024), DataSizeKB: Ptr(0)}); s != "" {
		t.Errorf("데이터 0이면 내역 생략해야 함: %q", s)
	}
	got := SizeBreakdown(App{SizeKB: Ptr(307200), DataSizeKB: Ptr(70 * 1024 * 1024)})
	if got != "번들 300 MB + 데이터 70.0 GB" {
		t.Errorf("sizeBreakdown = %q", got)
	}
}

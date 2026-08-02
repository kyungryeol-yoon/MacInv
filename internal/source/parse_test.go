package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyungryeol-yoon/macinv/internal/report"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s 읽기 실패: %v", name, err)
	}
	return data
}

func strv(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// mdls 1회 호출("키 = 값" 출력) 파싱 — 키 기반이라 속성 순서와 무관해야 한다(프로토타입 swap 버그 방지).
func TestParseMdlsKeyed(t *testing.T) {
	out := `kMDItemVersion             = "148.0.7778.179"
kMDItemDateAdded           = 2026-04-19 11:59:33 +0000
kMDItemCFBundleIdentifier  = "com.google.Chrome"
`
	m := parseMdlsKeyed(out)
	if m[attrBundleID] != "com.google.Chrome" {
		t.Errorf("bundle id = %q", m[attrBundleID])
	}
	if m[attrVersion] != "148.0.7778.179" {
		t.Errorf("version = %q (따옴표 제거돼야)", m[attrVersion])
	}
	if m[attrDateAdded] != "2026-04-19 11:59:33 +0000" {
		t.Errorf("date = %q", m[attrDateAdded])
	}
	// (null)/빈 값은 맵에서 제외.
	m2 := parseMdlsKeyed("kMDItemVersion = (null)\nkMDItemCFBundleIdentifier = \"x\"\n잘못된줄\n")
	if _, ok := m2[attrVersion]; ok {
		t.Errorf("(null)이 맵에 들어감")
	}
	if m2[attrBundleID] != "x" {
		t.Errorf("정상 값 유실: %v", m2)
	}
}

func TestParseBrewInfo(t *testing.T) {
	formulae, casks, err := parseBrewInfo(readFixture(t, "brew_info_v2.json"))
	if err != nil {
		t.Fatalf("parseBrewInfo 에러: %v", err)
	}
	if len(formulae) != 2 {
		t.Fatalf("formulae 개수 = %d, want 2", len(formulae))
	}
	git := formulae[0]
	if git.Name != "git" || strv(git.Version) != "2.53.0_1" {
		t.Errorf("git 파싱 오류: %+v", git)
	}
	if !git.InstalledOnRequest {
		t.Errorf("git.InstalledOnRequest = false, want true (leaf)")
	}
	if len(git.Dependencies) != 2 || git.Dependencies[0] != "pcre2" {
		t.Errorf("git.Dependencies = %v", git.Dependencies)
	}
	if len(casks) != 1 || casks[0].Name != "iterm2" || strv(casks[0].Version) != "3.5.6" {
		t.Errorf("cask 파싱 오류: %+v", casks)
	}
	// app_names: 프로토타입과 동일하게 nested-array/object 아티팩트는 추출하지 않음 → 빈 배열.
	if len(casks[0].AppNames) != 0 {
		t.Errorf("cask.AppNames = %v, want []", casks[0].AppNames)
	}
}

func TestApplyBrewOutdated(t *testing.T) {
	brew := &report.Brew{
		Formulae: []report.Formula{{Name: "git", Version: report.Ptr("2.53.0_1")}},
		Casks:    []report.Cask{{Name: "iterm2", Version: report.Ptr("3.5.6")}},
	}
	data := []byte(`{
		"formulae": [{"name": "git", "installed_versions": ["2.53.0_1"], "current_version": "2.55.0"}],
		"casks": [{"name": "iterm2", "installed_versions": ["3.5.6"], "current_version": "3.5.9"}]
	}`)
	applyBrewOutdated(data, brew)
	if !brew.Formulae[0].Outdated || strv(brew.Formulae[0].LatestVersion) != "2.55.0" {
		t.Errorf("git outdated 반영 실패: %+v", brew.Formulae[0])
	}
	if !brew.Casks[0].Outdated || strv(brew.Casks[0].LatestVersion) != "3.5.9" {
		t.Errorf("iterm2 outdated 반영 실패: %+v", brew.Casks[0])
	}
}

func TestParseNpmLs(t *testing.T) {
	data := []byte(`{"dependencies": {
		"npm": {"version": "10.9.7"},
		"@google/gemini-cli": {"version": "0.42.0"}
	}}`)
	pkgs := parseNpmLs(data)
	if len(pkgs) != 2 {
		t.Fatalf("패키지 개수 = %d, want 2", len(pkgs))
	}
	// 이름 정렬: @google/... 가 npm 보다 앞.
	if pkgs[0].Name != "@google/gemini-cli" {
		t.Errorf("정렬 오류: %v", pkgs[0].Name)
	}
}

func TestApplyNpmOutdated(t *testing.T) {
	npm := &report.Npm{Packages: []report.NpmPackage{
		{Name: "npm", Version: report.Ptr("10.9.7")},
		{Name: "corepack", Version: report.Ptr("0.34.6")},
	}}
	data := []byte(`{"npm": {"latest": "12.0.1"}, "corepack": {"latest": "0.35.0"}}`)
	applyNpmOutdated(data, npm)
	for _, p := range npm.Packages {
		if !p.Outdated {
			t.Errorf("%s outdated 반영 실패", p.Name)
		}
	}
	// 빈 입력은 아무것도 바꾸지 않아야 함.
	npm2 := &report.Npm{Packages: []report.NpmPackage{{Name: "npm"}}}
	applyNpmOutdated([]byte(""), npm2)
	if npm2.Packages[0].Outdated {
		t.Errorf("빈 입력이 outdated를 바꿈")
	}
}

func TestParsePipxList(t *testing.T) {
	data := []byte(`{"venvs": {
		"black": {"metadata": {"main_package": {"package_version": "24.1.0"}}},
		"httpie": {"metadata": {"main_package": {"package_version": "3.2.2"}}}
	}}`)
	out := parsePipxList(data)
	if len(out) != 2 || out[0].Name != "black" || strv(out[0].Version) != "24.1.0" {
		t.Errorf("pipx 파싱 오류: %+v", out)
	}
}

func TestParseCargoList(t *testing.T) {
	out := parseCargoList("ripgrep v14.1.0:\n    rg\nfd-find v10.2.0:\n    fd\n")
	if len(out) != 2 {
		t.Fatalf("cargo 개수 = %d, want 2", len(out))
	}
	if out[0].Name != "ripgrep" || strv(out[0].Version) != "14.1.0" {
		t.Errorf("cargo 파싱 오류: %+v", out[0])
	}
}

func TestParseMas(t *testing.T) {
	list := "497799835 Xcode (26.4.1)\n1295203466 Microsoft Remote Desktop (10.9.5)\n"
	outdated := "497799835 Xcode (26.4.0 -> 26.4.1)\n"
	mas := parseMas(list, outdated)
	if len(mas.Apps) != 2 {
		t.Fatalf("mas apps = %d, want 2", len(mas.Apps))
	}
	if mas.Apps[0].Name != "Xcode" || mas.Apps[0].Version != "26.4.1" {
		t.Errorf("mas list 파싱 오류: %+v", mas.Apps[0])
	}
	if len(mas.Outdated) != 1 || mas.Outdated[0].Version != "26.4.0" || mas.Outdated[0].LatestVersion != "26.4.1" {
		t.Errorf("mas outdated 파싱 오류: %+v", mas.Outdated)
	}
}

func TestParseLaunchdJSON(t *testing.T) {
	data := []byte(`{"Label": "com.logi.ghub", "ProgramArguments": ["/Applications/lghub.app/x"], "RunAtLoad": true}`)
	label, prog, runAtLoad, err := parseLaunchdJSON(data)
	if err != nil {
		t.Fatalf("parseLaunchdJSON 에러: %v", err)
	}
	if strv(label) != "com.logi.ghub" {
		t.Errorf("label = %s", strv(label))
	}
	// Program 없고 ProgramArguments 있으면 첫 인자를 program으로.
	if strv(prog) != "/Applications/lghub.app/x" {
		t.Errorf("program = %s", strv(prog))
	}
	if runAtLoad == nil || *runAtLoad != true {
		t.Errorf("runAtLoad = %v", runAtLoad)
	}
}

func TestSumDuKB(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want int
	}{
		{"빈 출력", "", 0},
		{"단일 경로", "12345\t/Users/x/Library/Caches/com.a.b\n", 12345},
		{
			"여러 경로 합산",
			"100\t/a\n200\t/b\n70453708\t/Users/x/Library/Application Support/Notion\n",
			70454008,
		},
		// du는 권한 없는 하위 경로를 stderr로 흘리고 나머지는 stdout에 그대로 낸다.
		{"불완전한 줄 무시", "100\t/a\ndu: /b: Permission denied\n50\t/c\n", 150},
		{"숫자가 아닌 첫 칸 무시", "abc\t/a\n30\t/b\n", 30},
		// 경로에 공백이 있어도 첫 칸만 쓰므로 안전하다.
		{"공백 포함 경로", "42\t/Users/x/Library/Application Support/Google Chrome\n", 42},
	}
	for _, tt := range tests {
		if got := sumDuKB(tt.out); got != tt.want {
			t.Errorf("%s: sumDuKB = %d, want %d", tt.name, got, tt.want)
		}
	}
}

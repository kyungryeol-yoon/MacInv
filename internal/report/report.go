// Package report는 인벤토리 JSON 스키마(프로토타입 mac_inventory.py 준수)와
// 리포트 조립 로직을 담는다. null이 될 수 있는 필드는 포인터로 두어 JSON에서 null로 나가게 한다.
package report

// Report는 최상위 리포트다.
type Report struct {
	GeneratedAt     string           `json:"generated_at"`
	Hostname        string           `json:"hostname"`
	OutdatedSkipped bool             `json:"outdated_skipped"`
	Sections        Sections         `json:"sections"`
	Runtimes        []Runtime        `json:"runtimes,omitempty"`
	VersionManagers []VersionManager `json:"version_managers,omitempty"`
	Errors          []string         `json:"errors"`
}

// Sections는 소스별 스캔 결과 묶음이다. 미설치 소스는 nil(JSON null)로 나간다.
type Sections struct {
	Apps    []App         `json:"apps"`
	Brew    *Brew         `json:"brew"`
	Npm     *Npm          `json:"npm"`
	Pipx    []NameVersion `json:"pipx"`
	Cargo   []NameVersion `json:"cargo"`
	Mas     *Mas          `json:"mas"`
	GoBin   []GoBinItem   `json:"go_bin"`
	Launchd []LaunchdItem `json:"launchd"`
}

// App은 GUI 앱 한 개다.
type App struct {
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	BundleID  *string `json:"bundle_id"`
	Version   *string `json:"version"`
	DateAdded *string `json:"date_added"`
	SizeKB    *int    `json:"size_kb"`
	// DataSizeKB는 ~/Library 등지에 쌓인 앱 데이터/캐시 용량이다(번들 밖).
	// 앱 번들은 수백 MB인데 데이터가 수십 GB인 경우가 흔해, 실제 디스크 점유는 이쪽이 좌우한다.
	DataSizeKB *int `json:"data_size_kb,omitempty"`
}

// TotalSizeKB는 번들+데이터 합계다. 둘 다 없으면 nil.
func (a App) TotalSizeKB() *int {
	if a.SizeKB == nil && a.DataSizeKB == nil {
		return nil
	}
	total := 0
	if a.SizeKB != nil {
		total += *a.SizeKB
	}
	if a.DataSizeKB != nil {
		total += *a.DataSizeKB
	}
	return &total
}

// Brew는 Homebrew 전체 결과다.
type Brew struct {
	Prefix   *string   `json:"prefix"`
	Formulae []Formula `json:"formulae"`
	Casks    []Cask    `json:"casks"`
}

// Formula는 brew formula 한 개다.
type Formula struct {
	Name                 string   `json:"name"`
	Version              *string  `json:"version"`
	InstalledOnRequest   bool     `json:"installed_on_request"`
	InstalledAsDependency bool    `json:"installed_as_dependency"`
	Dependencies         []string `json:"dependencies"`
	LatestVersion        *string  `json:"latest_version"`
	Outdated             bool     `json:"outdated"`
}

// Cask는 brew cask 한 개다.
type Cask struct {
	Name          string   `json:"name"`
	Version       *string  `json:"version"`
	AppNames      []string `json:"app_names"`
	LatestVersion *string  `json:"latest_version"`
	Outdated      bool     `json:"outdated"`
}

// Npm은 npm 글로벌 결과다.
type Npm struct {
	Root     *string       `json:"root"`
	Packages []NpmPackage  `json:"packages"`
}

// NpmPackage는 npm 글로벌 패키지 한 개다.
type NpmPackage struct {
	Name          string  `json:"name"`
	Version       *string `json:"version"`
	LatestVersion *string `json:"latest_version"`
	Outdated      bool    `json:"outdated"`
}

// NameVersion은 pipx/cargo처럼 이름+버전만 있는 항목이다.
type NameVersion struct {
	Name    string  `json:"name"`
	Version *string `json:"version"`
}

// Mas는 App Store(mas) 결과다.
type Mas struct {
	Apps     []MasApp      `json:"apps"`
	Outdated []MasOutdated `json:"outdated"`
}

// MasApp은 App Store 설치 앱 한 개다.
type MasApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MasOutdated는 업데이트 대기 중인 App Store 앱이다.
type MasOutdated struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	LatestVersion string `json:"latest_version"`
}

// GoBinItem은 ~/go/bin의 실행 파일 한 개다.
type GoBinItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// LaunchdItem은 LaunchAgent/Daemon plist 한 개다.
type LaunchdItem struct {
	File       string  `json:"file"`
	Kind       string  `json:"kind"`
	Label      *string `json:"label"`
	Program    *string `json:"program"`
	RunAtLoad  *bool   `json:"run_at_load"`
	ParseError *string `json:"parse_error,omitempty"`
}

// Ptr은 값 v의 포인터를 돌려주는 제네릭 헬퍼다(null 가능 필드 채울 때 사용).
func Ptr[T any](v T) *T { return &v }

// PtrOrNil은 빈 문자열이면 nil, 아니면 포인터를 돌려준다.
func PtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

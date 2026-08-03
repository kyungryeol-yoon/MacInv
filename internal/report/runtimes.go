package report

import (
	"fmt"
	"time"
)

// 이 파일은 런타임/소유권 추적 관련 데이터 타입만 정의한다.
// 실제 수집 로직은 internal/source에 있고(source가 report를 import), 관리 주체 문자열
// 상수(brew/npm/nvm/...)도 source에 있다. 여기서는 순수 데이터 모델만 둔다.

// Origin은 어떤 바이너리 경로 하나의 소유권 역추적 결과다.
type Origin struct {
	Path     string `json:"path"`     // PATH에 나타난 원래 경로
	Resolved string `json:"resolved"` // 심볼릭 링크 해소 후 실제 경로
	Manager  string `json:"manager"`  // brew|npm|cargo|pipx|go|system|manual|nvm|pyenv|...
	Package  string `json:"package"`  // 소속 패키지/포뮬러 이름(있으면)
	Active   bool   `json:"active"`   // PATH상 가장 먼저 발견되어 실제 실행되는 경로인지
}

// Runtime은 개발 언어/런타임 하나의 현황이다.
//
// EOL·최신버전 필드는 endoflife.date 조회 결과다(source.EnrichEOL이 채운다).
// 조회에 실패하거나 --offline이면 빈 값으로 남는다 — **없어도 설치 현황은 그대로 보여야 한다**.
type Runtime struct {
	Name      string   `json:"name"`
	Installed bool     `json:"installed"`
	Version   string   `json:"version"`  // 활성 버전(추출값)
	Origins   []Origin `json:"origins"`  // 발견된 모든 경로(관리 주체 포함)
	Conflict  bool     `json:"conflict"` // 서로 다른 실제 경로가 2개 이상 → PATH 충돌 가능

	// 아래는 전부 부가 정보다. 비어 있으면 표시를 생략한다.
	// InstalledVersion은 Version과 같은 값이다. Version은 기존 스키마 호환을 위해 남기고,
	// 이름이 분명한 installed_version을 함께 내보낸다(latest_version과 짝을 이루게).
	InstalledVersion string `json:"installed_version,omitempty"`
	// Manager는 활성 경로의 관리 주체다(origins[0].manager와 같은 값 — 최상위에서 바로 읽게).
	Manager string `json:"manager,omitempty"`
	// LatestVersion은 **같은 사이클 안의** 최신 패치다(안전한 업그레이드로 도달하는 버전).
	LatestVersion string `json:"latest_version,omitempty"`
	// LatestCycle은 나와 있는 **가장 최신 릴리스 사이클**이다(3.14, 26 …).
	// LatestVersion과 구분하는 이유: 패치 업그레이드와 메이저 업그레이드는 위험도가 전혀 다르다.
	LatestCycle string `json:"latest_cycle,omitempty"`
	EOLCycle    string `json:"eol_cycle,omitempty"`  // 설치 버전이 속한 릴리스 사이클(3.9, 26, 1.26 …)
	EOLDate     string `json:"eol_date,omitempty"`   // 지원 종료일(YYYY-MM-DD). 미정이면 빈 값
	EOLStatus   string `json:"eol_status,omitempty"` // supported|soon|passed|rolling
	// UpdateStatus는 current|patch|major. major는 **자동 실행 대상이 아니다** —
	// 런타임 메이저 업그레이드는 프로젝트를 깨뜨린다.
	UpdateStatus string `json:"update_status,omitempty"`
	// SuggestedCommand는 올릴 때 쓸 명령이다. 표시만 하고 실행하지 않는다.
	SuggestedCommand string `json:"suggested_command,omitempty"`
}

// VersionManager는 감지된 버전 매니저와 그것이 관리 중인 버전 목록이다.
type VersionManager struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// EOLStatus 값.
const (
	EOLUnknown   = ""          // 조회 못 함(오프라인 등)
	EOLSupported = "supported" // 지원 중
	EOLSoon      = "soon"      // 곧 종료(12개월 이내)
	EOLPassed    = "passed"    // 이미 종료
	EOLRolling   = "rolling"   // 종료일이 정해지지 않음(go/rust처럼 굴러가는 릴리스)
)

// UpdateStatus 값.
const (
	UpdateUnknown = ""        // 판단 못 함
	UpdateCurrent = "current" // 현재 사이클의 최신 패치
	UpdatePatch   = "patch"   // 같은 메이저 안의 패치가 있음 — 안전하다
	UpdateMajor   = "major"   // 더 최신 메이저가 있음 — 깨질 수 있어 자동 실행 대상 아님
)

// EOLBadge는 지원 종료 상태 배지다(순수 함수 — CLI/TUI 공용).
// now를 받아 "몇 개월 뒤"를 계산한다. 상태를 모르면 빈 문자열(아무 말도 하지 않는다).
func EOLBadge(status, eolDate string, now time.Time) string {
	switch status {
	case EOLPassed:
		if eolDate != "" {
			return "[EOL 지남 " + eolDate + "]"
		}
		return "[EOL 지남]"
	case EOLSoon:
		if t, err := time.Parse("2006-01-02", eolDate); err == nil {
			return fmt.Sprintf("[%d개월 뒤 종료]", monthsUntil(now, t))
		}
		return "[곧 종료]"
	case EOLSupported, EOLRolling:
		// rolling은 종료일 미정(go/rust처럼 굴러가는 릴리스) — 사용자에겐 둘 다 "지원 중"이다.
		return "[지원 중]"
	}
	return ""
}

// monthsUntil은 now에서 t까지 남은 개월 수다(최소 0). 순수 함수.
func monthsUntil(now, t time.Time) int {
	m := int(t.Sub(now).Hours() / 24 / 30.44)
	if m < 0 {
		return 0
	}
	return m
}

// UpdateBadge는 업데이트 상태 배지다(순수 함수 — CLI/TUI 공용).
// 메이저는 "최신은 X"로만 알린다 — 올리라는 지시가 아니라 판단 근거다.
func UpdateBadge(status, latestCycle string) string {
	switch status {
	case UpdateCurrent:
		return "[현재]"
	case UpdatePatch:
		return "[안전한 패치 있음]"
	case UpdateMajor:
		if latestCycle != "" {
			return "[메이저 최신은 " + latestCycle + "]"
		}
		return "[메이저 최신 있음]"
	}
	return ""
}

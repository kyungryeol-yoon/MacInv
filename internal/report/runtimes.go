package report

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
type Runtime struct {
	Name      string   `json:"name"`
	Installed bool     `json:"installed"`
	Version   string   `json:"version"`  // 활성 버전(추출값)
	Origins   []Origin `json:"origins"`  // 발견된 모든 경로(관리 주체 포함)
	Conflict  bool     `json:"conflict"` // 서로 다른 실제 경로가 2개 이상 → PATH 충돌 가능
}

// VersionManager는 감지된 버전 매니저와 그것이 관리 중인 버전 목록이다.
type VersionManager struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

package clean

import (
	"testing"
)

// npm 셀프 업데이트는 npx 경유 명령이어야 한다(자기 파일 삭제로 인한 MODULE_NOT_FOUND 방지).
func TestUpgradeNpmSelfUsesNpx(t *testing.T) {
	// BuildUpgradePlan은 시스템 조회를 하므로, 명령 생성 규칙만 직접 검증한다.
	tgt := npmUpgradeTarget("npm")
	if len(tgt.Argv) != 1 || ShellQuote(tgt.Argv[0]) != "npx -y npm@latest install -g npm@latest" {
		t.Errorf("npm 셀프 업데이트 명령 오류: %v", tgt.Argv)
	}
	if len(tgt.Warnings) == 0 {
		t.Errorf("npm 셀프 업데이트 경고가 없음")
	}

	other := npmUpgradeTarget("@google/gemini-cli")
	if len(other.Argv) != 1 || ShellQuote(other.Argv[0]) != "npm install -g @google/gemini-cli@latest" {
		t.Errorf("일반 npm 패키지 명령 오류: %v", other.Argv)
	}
}

// 업그레이드 타깃은 상향된 타임아웃(30분)을 가진다 — 대형 다운로드 대비, 삭제는 기본 유지.
func TestUpgradeTargetTimeout(t *testing.T) {
	if got := npmUpgradeTarget("npm").CmdTimeoutSec; got != upgradeTimeoutSec {
		t.Errorf("npm 업그레이드 타임아웃 = %d, want %d", got, upgradeTimeoutSec)
	}
	if upgradeTimeoutSec != 1800 {
		t.Errorf("upgradeTimeoutSec = %d, want 1800", upgradeTimeoutSec)
	}
}

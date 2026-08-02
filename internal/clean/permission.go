package clean

import (
	"path/filepath"
	"syscall"
)

// bundleGlobs는 /Library/... 경로도 [확실]로 잡지만, 그쪽은 보통 root 소유라
// 휴지통 이동이 권한 오류로 실패하고 로그에 ✗만 남는다. 계획 단계에서 미리 갈라놓아
// "지웠다고 생각했는데 안 지워진" 상태를 없앤다.

// canTrash는 경로를 휴지통으로 옮길 수 있는지 본다.
// 항목 자체가 아니라 **부모 디렉토리**의 쓰기 권한이 기준이다 — 휴지통 이동은 부모에서의 unlink다.
func canTrash(path string) bool {
	const wOK = 0x02
	return syscall.Access(filepath.Dir(path), wOK) == nil
}

// splitByWritability는 휴지통 이동이 가능한 경로와 관리자 권한이 필요한 경로를 나눈다
// (순수 함수 — writable을 주입해 테스트한다).
func splitByWritability(matches []PathMatch, writable func(string) bool) (ok, needAdmin []PathMatch) {
	for _, m := range matches {
		if writable(m.Path) {
			ok = append(ok, m)
		} else {
			needAdmin = append(needAdmin, m)
		}
	}
	return ok, needAdmin
}

// markNeedsAdmin은 계획 전체에서 권한이 부족한 경로를 TrashPaths에서 빼 NeedsAdmin으로 옮긴다.
// 계획 수립의 마지막 단계로 부른다(회수 예상 계산 전 — 못 지우는 건 회수량에도 넣지 않는다).
func markNeedsAdmin(plan *Plan) {
	for i := range plan.Targets {
		t := &plan.Targets[i]
		trash, needAdmin := splitByWritability(t.TrashPaths, canTrash)
		if len(needAdmin) == 0 {
			continue
		}
		t.TrashPaths = trash
		t.NeedsAdmin = append(t.NeedsAdmin, needAdmin...)
		t.Suggestions = append(t.Suggestions,
			"권한 필요 항목은 자동 삭제하지 않습니다 — Finder에서 직접 옮기거나 sudo로 처리하세요.")
	}
}

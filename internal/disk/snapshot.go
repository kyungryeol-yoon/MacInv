package disk

import (
	"context"
	"strconv"
	"strings"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
)

// APFS 로컬 스냅샷은 "지웠는데 용량이 안 늘어나는" 가장 흔한 이유다.
// Time Machine이 로컬에 남긴 스냅샷이 지운 파일의 블록을 붙들고 있어서,
// df의 여유 공간이 실제 여유(purgeable 포함)와 크게 어긋난다.
//
// **삭제하지 않는다.** tmutil deletelocalsnapshots는 백업 복원 지점을 없애는 행위라
// 이 도구가 대신 결정할 일이 아니다. 원인을 이름 붙여 알려주는 것까지만 한다.

// SnapshotReport는 로컬 스냅샷 현황이다.
type SnapshotReport struct {
	Names []string `json:"names"`
	// AvailKB/UsedKB는 df 기준 값이다(스냅샷이 붙든 공간은 Avail에 안 잡힌다).
	AvailKB int `json:"avail_kb"`
	UsedKB  int `json:"used_kb"`
}

// ScanSnapshots는 로컬 스냅샷 목록과 df 값을 모은다(읽기 전용).
// 조회에 실패하면 nil을 돌려준다 — 이건 부가 정보라 실패가 커맨드를 막으면 안 된다.
func ScanSnapshots(ctx context.Context, run *iexec.Runner) *SnapshotReport {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 30}, "tmutil", "listlocalsnapshots", "/")
	if err != nil {
		return nil
	}
	rep := &SnapshotReport{Names: ParseSnapshots(out)}

	// 데이터 볼륨 기준으로 봐야 한다(/ 는 읽기 전용 시스템 스냅샷이라 항상 거의 안 변한다).
	if dfOut, err := run.Run(ctx, iexec.Opt{TimeoutSec: 30}, "df", "-k", "/System/Volumes/Data"); err == nil {
		rep.UsedKB, rep.AvailKB = ParseDF(dfOut)
	}
	return rep
}

// ParseSnapshots는 tmutil listlocalsnapshots 출력에서 스냅샷 이름을 뽑는다(순수 함수 — 테스트 대상).
// 첫 줄은 "Snapshots for disk /:" 같은 헤더라 건너뛴다.
func ParseSnapshots(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "com.apple.TimeMachine.") {
			names = append(names, line)
		}
	}
	return names
}

// ParseDF는 df -k 출력에서 (사용 KB, 여유 KB)를 뽑는다(순수 함수 — 테스트 대상).
// 헤더 다음 줄의 3번째/4번째 필드다. 파싱 실패는 0,0.
func ParseDF(out string) (usedKB, availKB int) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	f := strings.Fields(lines[len(lines)-1])
	if len(f) < 4 {
		return 0, 0
	}
	usedKB, _ = strconv.Atoi(f[2])
	availKB, _ = strconv.Atoi(f[3])
	return usedKB, availKB
}

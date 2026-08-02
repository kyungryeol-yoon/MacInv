package disk

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/kyungryeol-yoon/macinv/internal/clean"
	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
)

// KindDisk는 디스크 정리 타깃이다(clean.Plan 안에서 구분용).
const KindDisk = "disk"

// cleanupTimeoutSec는 정리 명령 타임아웃이다. brew cleanup / go clean -modcache는
// 수만 개 파일을 지워 오래 걸린다(삭제 기본 300초로는 부족).
const cleanupTimeoutSec = 900

// BuildCleanupPlan은 측정 결과에서 정리 계획을 만든다(읽기 전용 — 아무것도 지우지 않는다).
//
// **계획/실행은 clean 패키지 것을 그대로 쓴다.** 안전 규칙(dry-run 기본, rm 금지,
// 블랙리스트 재확인, 권한 분리, 확인 절차)이 두 벌이 되면 안 되기 때문이다.
// 여기서 하는 일은 "어떤 위치를 담을지" 고르는 것뿐이다.
//
// selected가 nil이 아니면 그 ID만 담는다(TUI 멀티선택). includeExpensive가 false면
// expensive 등급은 제외한다(clean의 --include-low와 같은 패턴). never는 언제나 제외.
func BuildCleanupPlan(res *Result, selected map[string]bool, includeExpensive bool) *clean.Plan {
	plan := &clean.Plan{Query: "디스크 정리", Verb: "정리", Icon: "🧽"}
	for _, e := range res.Entries {
		if selected != nil && !selected[e.ID] {
			continue
		}
		if !Cleanable(e.Tier, includeExpensive) {
			continue
		}
		if len(e.Cleanup) == 0 && !e.Trash {
			continue // 리포트 전용 위치(네이티브 명령 없음)
		}
		t := clean.Target{
			Kind:          KindDisk,
			Name:          e.Name + " " + TierLabel(e.Tier),
			CmdTimeoutSec: cleanupTimeoutSec,
		}
		t.Argv = append(t.Argv, e.Cleanup...)
		if e.Trash {
			// 네이티브 명령이 없는 위치(DerivedData 등)는 휴지통 이동으로 정리한다 — rm은 쓰지 않는다.
			for _, p := range e.Paths {
				t.TrashPaths = append(t.TrashPaths, clean.PathMatch{
					Path: p, Confidence: clean.ConfHigh, Reason: e.Name + " (빌드 산출물 · 재생성됨)",
					SizeKB: e.SizeKB,
				})
			}
			t.Suggestions = append(t.Suggestions,
				"휴지통으로 옮기므로 비우기 전까지는 디스크가 실제로 비지 않습니다.")
		}
		if !e.Trash {
			// 명령이 지우는 몫은 휴지통 경로가 아니라 위치 용량이다 — 회수 예상에 따로 더한다.
			plan.ExtraReclaimKB += e.SizeKB
		}
		if e.Note != "" {
			t.Suggestions = append(t.Suggestions, e.Note)
		}
		if e.Tier == TierExpensive {
			// 재다운로드가 비싼 항목은 승인 뒤 한 번 더 확인받는다(GUI 앱 삭제와 같은 취급).
			plan.RequireConfirm = true
		}
		if e.Orphan {
			t.Warnings = append(t.Warnings,
				"고아 캐시 — '"+e.Tool+"'가 인벤토리에 없습니다(도구를 지운 뒤 남은 것으로 보입니다).")
		}
		plan.Targets = append(plan.Targets, t)
	}
	return plan
}

// ---- Ollama 모델 (개별 선택) ----------------------------------------------
//
// 모델은 위치 단위로 뭉뚱그려 지울 수 없다. 하나가 수 GB이고 재다운로드가 비싸서
// "어떤 모델이 몇 GB인지" 보여주고 고르게 해야 한다.

// OllamaModel은 설치된 모델 하나다.
type OllamaModel struct {
	Name   string `json:"name"`
	SizeKB int    `json:"size_kb"`
}

// ollamaListLine은 `ollama list`의 한 줄에서 이름과 크기를 뽑는다.
// 형식: "llama3:8b   365c0bd3c000   4.7 GB   3 days ago"
var ollamaListLine = regexp.MustCompile(`^(\S+)\s+\S+\s+([\d.]+)\s*([KMGT]B)\s`)

// ScanOllamaModels는 설치된 ollama 모델과 용량을 조회한다(읽기 전용).
// ollama가 없으면 nil(조용한 스킵 — 미설치는 에러가 아니다).
func ScanOllamaModels(ctx context.Context, run *iexec.Runner) []OllamaModel {
	out, err := run.Run(ctx, iexec.Opt{TimeoutSec: 30}, "ollama", "list")
	if err != nil {
		return nil
	}
	return ParseOllamaList(out)
}

// ParseOllamaList는 `ollama list` 출력을 파싱한다(순수 함수 — 테스트 대상).
func ParseOllamaList(out string) []OllamaModel {
	var models []OllamaModel
	for _, line := range strings.Split(out, "\n") {
		m := ollamaListLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		models = append(models, OllamaModel{Name: m[1], SizeKB: humanToKB(m[2], m[3])})
	}
	return models
}

// humanToKB는 "4.7", "GB" 같은 값을 KB로 바꾼다(순수 함수).
func humanToKB(num, unit string) int {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case "KB":
		return int(v)
	case "MB":
		return int(v * 1024)
	case "GB":
		return int(v * 1024 * 1024)
	case "TB":
		return int(v * 1024 * 1024 * 1024)
	}
	return 0
}

// BuildOllamaPlan은 고른 모델만 지우는 계획을 만든다(순수 함수 — 테스트 대상).
// 모델 하나당 `ollama rm <model>` 하나 — 부분 실패해도 나머지는 지워진다.
func BuildOllamaPlan(models []OllamaModel, selected map[string]bool) *clean.Plan {
	plan := &clean.Plan{Query: "Ollama 모델 삭제", Verb: "삭제", Icon: "🧹"}
	for _, m := range models {
		if !selected[m.Name] {
			continue
		}
		plan.Targets = append(plan.Targets, clean.Target{
			Kind:          KindDisk,
			Name:          m.Name,
			Argv:          [][]string{{"ollama", "rm", m.Name}},
			CmdTimeoutSec: cleanupTimeoutSec,
			Warnings:      []string{"재다운로드에 시간이 걸립니다(약 " + humanKB(m.SizeKB) + ")."},
		})
	}
	return plan
}

func humanKB(kb int) string {
	switch {
	case kb >= 1024*1024:
		return strconv.FormatFloat(float64(kb)/1024/1024, 'f', 1, 64) + " GB"
	case kb >= 1024:
		return strconv.Itoa(kb/1024) + " MB"
	default:
		return strconv.Itoa(kb) + " KB"
	}
}

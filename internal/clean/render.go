package clean

import (
	"fmt"
	"strings"
)

// PlanLine은 삭제 계획의 한 줄이다(CLI 텍스트·UI가 공유하는 구조화 표현).
// Toggleable이 true면 그 줄은 [확신낮음] 잔여 경로로, 사용자가 켜야만 삭제 대상이 된다.
type PlanLine struct {
	Text       string // 렌더용 텍스트(들여쓰기 포함, 배지 제외)
	Kind       string // "planwarning"|"target"|"bundle"|"warning"|"command"|"path"|"needadmin"|"excluded"|"suggestion"
	Confidence string // Kind=="path"일 때만: high|medium|low
	Path       string // Kind=="path"|"excluded"일 때 실제 경로
	Toggleable bool   // 확신낮음 경로 → 체크박스 대상
}

// PlanLines는 계획을 구조화된 줄 목록으로 만든다(CLI/UI 공용 단일 소스).
func PlanLines(p *Plan) []PlanLine {
	var lines []PlanLine
	// 계획 전체 경고를 맨 앞에 — 타깃 목록에 묻히면 승인 전에 못 본다.
	// PlanLines에 넣어야 CLI 텍스트와 UI가 같은 것을 보여준다.
	for _, w := range p.Warnings {
		lines = append(lines, PlanLine{Kind: "planwarning", Text: "⚠️  " + w})
	}
	for _, t := range p.Targets {
		lines = append(lines, PlanLine{Kind: "target", Text: fmt.Sprintf("▸ %s (%s)", t.Name, KindLabel(t.Kind))})
		if t.BundleID != "" {
			lines = append(lines, PlanLine{Kind: "bundle", Text: "  Bundle ID: " + t.BundleID})
		}
		for _, w := range t.Warnings {
			lines = append(lines, PlanLine{Kind: "warning", Text: "  ⚠️  " + w})
		}
		for _, argv := range t.Argv {
			lines = append(lines, PlanLine{Kind: "command", Text: "  $ " + ShellQuote(argv)})
		}
		for _, pm := range t.TrashPaths {
			lines = append(lines, PlanLine{
				Kind: "path", Text: "  " + ConfBadge(pm.Confidence) + " " + pm.Path + "  — " + pm.Reason,
				Confidence: pm.Confidence, Path: pm.Path, Toggleable: pm.Confidence == ConfLow,
			})
		}
		for _, pm := range t.NeedsAdmin {
			lines = append(lines, PlanLine{
				Kind: "needadmin", Text: "  🔑 권한 필요(수동): " + pm.Path + "  — " + pm.Reason,
				Confidence: pm.Confidence, Path: pm.Path,
			})
		}
		for _, e := range t.Excluded {
			lines = append(lines, PlanLine{Kind: "excluded", Text: "  🔒 보존: " + e.Path + "  — " + e.Reason, Path: e.Path})
		}
		for _, s := range t.Suggestions {
			lines = append(lines, PlanLine{Kind: "suggestion", Text: "  💡 " + s})
		}
	}
	return lines
}

// RenderPlanText는 계획을 CLI 블록 문자열로 렌더링한다(UI와 동일 내용을 보장).
func RenderPlanText(p *Plan) string {
	line := strings.Repeat("─", 62)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n %s %s 계획: '%s'\n%s\n", line, p.icon(), p.verb(), p.Query, line)
	for _, l := range PlanLines(p) {
		if l.Kind == "target" {
			b.WriteString("\n")
		}
		b.WriteString("  " + l.Text + "\n")
	}
	// 회수 예상은 [확신낮음] 제외 기준(= --apply 기본값)으로 낸다.
	if rl := ReclaimLine(p, nil); rl != "" {
		fmt.Fprintf(&b, "\n  %s\n", rl)
	}
	return b.String()
}

// shellSafe는 따옴표 없이 그대로 보여도 되는 문자들이다.
const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/-=:@%+,"

// ShellQuote는 argv를 사람이 읽을 수 있는 한 줄 명령으로 만든다(순수 함수 — 테스트 대상).
// **표시 전용**이다: 실행은 언제나 argv 배열 그대로라 이 문자열을 다시 파싱하지 않는다.
// 공백/따옴표가 든 인자는 작은따옴표로 감싸 사용자가 복사해 붙여도 같은 뜻이 되게 한다.
func ShellQuote(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(a string) string {
	if a == "" {
		return "''"
	}
	if strings.IndexFunc(a, func(r rune) bool { return !strings.ContainsRune(shellSafe, r) }) < 0 {
		return a
	}
	// 작은따옴표 안에서는 모든 문자가 리터럴이다. 작은따옴표 자체만 '\'' 로 빠져나간다.
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// KindLabel은 타깃 종류의 한국어 라벨이다(CLI/UI 공용).
func KindLabel(kind string) string {
	switch kind {
	case KindApp:
		return "GUI 앱"
	case KindBrewFormula:
		return "brew formula"
	case KindBrewCask:
		return "brew cask"
	case KindNpm:
		return "npm 전역"
	case KindPipx:
		return "pipx"
	case KindCargo:
		return "cargo"
	case KindGoBin:
		return "go bin"
	case KindMas:
		return "App Store"
	case KindOrphan:
		return "고아 잔여물"
	default:
		return kind
	}
}

// ConfBadge는 확신도 배지다(CLI/UI 공용).
func ConfBadge(conf string) string {
	switch conf {
	case ConfMedium:
		return "[확신중간]"
	case ConfLow:
		return "[확신낮음]"
	default:
		return "[확실]"
	}
}

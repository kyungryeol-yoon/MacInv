// Package exec는 외부 네이티브 명령을 안전하게 호출하는 공통 헬퍼다.
// 옛 Python 프로토타입의 run()/run_json() 규칙을 계승한다:
//   - 실패해도 panic 하지 않고 (출력, error)를 돌려준다.
//   - 경고는 전역 Recorder에 모아 리포트의 errors 필드로 내보낸다.
//   - npm처럼 문제가 없어도 exit 1을 내는 명령을 위해 허용 종료코드를 지정할 수 있다.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func secDuration(sec int) time.Duration { return time.Duration(sec) * time.Second }

// Recorder는 스캔 도중 발생한 비치명적 경고를 모은다. 동시성 안전.
type Recorder struct {
	mu   sync.Mutex
	msgs []string
}

// Add는 경고 한 건을 기록한다.
func (r *Recorder) Add(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg)
}

// Messages는 기록된 경고 사본을 돌려준다.
func (r *Recorder) Messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.msgs))
	copy(out, r.msgs)
	return out
}

// Runner는 timeout과 경고 수집을 묶은 실행기다. 실제 timeout은 Run 호출마다 개별 적용된다.
type Runner struct {
	Rec      *Recorder
	DefTmout int // 기본 timeout(초)
}

// NewRunner는 주어진 경고 Recorder와 기본 timeout으로 Runner를 만든다.
func NewRunner(rec *Recorder, defaultTimeoutSec int) *Runner {
	if defaultTimeoutSec <= 0 {
		defaultTimeoutSec = 120
	}
	return &Runner{Rec: rec, DefTmout: defaultTimeoutSec}
}

// Opt는 Run 호출 옵션이다.
type Opt struct {
	// OKCodes에 없는 종료코드는 실패로 본다. 비어 있으면 {0}.
	OKCodes []int
	// TimeoutSec가 0이면 Runner.DefTmout을 쓴다.
	TimeoutSec int
	// Strict가 true면 stdout이 있어도 비허용 종료코드는 실패다.
	// 스캔(읽기)은 관대하게, 삭제/업데이트(변경)는 엄격하게 — Execute 경로에서 사용.
	Strict bool
}

func (o Opt) okCodes() []int {
	if len(o.OKCodes) == 0 {
		return []int{0}
	}
	return o.OKCodes
}

// Run은 명령을 실행하고 stdout을 돌려준다.
// 명령 자체가 없으면(FileNotFound) 경고 없이 ("", ErrNotFound)를 돌려준다 — "미설치"는 정상 상황이다.
// 그 외 실패는 경고를 기록하고 에러를 돌려준다. 단, 종료코드가 허용 밖이라도 stdout이 있으면 성공으로 본다
// (프로토타입 run()의 동작과 동일).
func (r *Runner) Run(ctx context.Context, opt Opt, name string, args ...string) (string, error) {
	timeout := opt.TimeoutSec
	if timeout <= 0 {
		timeout = r.DefTmout
	}
	cctx, cancel := context.WithTimeout(ctx, secDuration(timeout))
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	label := name + " " + strings.Join(args, " ")

	if err != nil {
		// 실행 파일 자체가 없음 → 미설치. 조용히 스킵.
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNotFound
		}
		if cctx.Err() == context.DeadlineExceeded {
			r.Rec.Add(fmt.Sprintf("%s: timeout", label))
			return "", cctx.Err()
		}
		// 종료코드 오류: 허용 코드거나, 허용 밖이어도 stdout이 있으면 성공 취급.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			if containsInt(opt.okCodes(), code) || (!opt.Strict && stdout.Len() > 0) {
				return stdout.String(), nil
			}
			detail := truncate(stderr.String(), 200)
			if detail == "" {
				detail = truncate(stdout.String(), 200) // stderr가 비면 stdout에서라도 원인 확보
			}
			r.Rec.Add(fmt.Sprintf("%s: exit %d %s", label, code, detail))
			if detail != "" {
				// 호출자(CLI/UI 로그)가 원인을 볼 수 있게 상세를 에러에 포함한다.
				return "", fmt.Errorf("exit %d: %s", code, detail)
			}
			return "", err
		}
		r.Rec.Add(fmt.Sprintf("%s: %v", label, err))
		return "", err
	}
	return stdout.String(), nil
}

// RunCombined는 stdout과 stderr를 합쳐 돌려준다(버전 문자열이 stderr로 나오는 java/swift 등 대응).
// 종료코드는 관대하게 처리한다 — 버전 조회는 non-zero를 내도 유효 출력을 주는 경우가 많다.
// 실행 파일 자체가 없으면 ErrNotFound를 돌려준다(경고 없음).
func (r *Runner) RunCombined(ctx context.Context, opt Opt, name string, args ...string) (string, error) {
	timeout := opt.TimeoutSec
	if timeout <= 0 {
		timeout = r.DefTmout
	}
	cctx, cancel := context.WithTimeout(ctx, secDuration(timeout))
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNotFound
		}
		if cctx.Err() == context.DeadlineExceeded {
			r.Rec.Add(fmt.Sprintf("%s %s: timeout", name, strings.Join(args, " ")))
			return buf.String(), cctx.Err()
		}
		// 종료코드 오류라도 출력이 있으면 그대로 돌려준다(best effort).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && buf.Len() > 0 {
			return buf.String(), nil
		}
		return buf.String(), err
	}
	return buf.String(), nil
}

// RunJSON은 Run 결과를 JSON으로 파싱해 v에 담는다.
func (r *Runner) RunJSON(ctx context.Context, opt Opt, v any, name string, args ...string) error {
	out, err := r.Run(ctx, opt, name, args...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return ErrEmpty
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		r.Rec.Add(fmt.Sprintf("%s: JSON 파싱 실패", name+" "+strings.Join(args, " ")))
		return err
	}
	return nil
}

// ErrNotFound는 명령 실행 파일이 없을 때(미설치) 반환된다.
var ErrNotFound = errors.New("command not found")

// ErrEmpty는 명령 출력이 비었을 때 반환된다.
var ErrEmpty = errors.New("empty output")

func containsInt(s []int, x int) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

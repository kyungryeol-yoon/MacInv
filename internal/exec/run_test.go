package exec

import (
	"context"
	"strings"
	"testing"
)

// 실패 시 에러 메시지에 stderr 원인이 포함되어야 한다(CLI/UI 로그에 원인 표시용).
func TestRunErrorIncludesStderr(t *testing.T) {
	rec := &Recorder{}
	r := NewRunner(rec, 10)
	_, err := r.Run(context.Background(), Opt{}, "sh", "-c", "echo boom-cause >&2; exit 3")
	if err == nil {
		t.Fatal("실패해야 하는데 err=nil")
	}
	if !strings.Contains(err.Error(), "boom-cause") {
		t.Errorf("err에 stderr 원인 없음: %v", err)
	}
	if !strings.Contains(err.Error(), "exit 3") {
		t.Errorf("err에 종료코드 없음: %v", err)
	}
}

// stdout이 있으면 비허용 종료코드라도 성공(프로토타입 규칙 회귀 방지) — 단 스캔(비 Strict)에서만.
func TestRunToleratesExitWithStdout(t *testing.T) {
	rec := &Recorder{}
	r := NewRunner(rec, 10)
	out, err := r.Run(context.Background(), Opt{}, "sh", "-c", "echo data; exit 1")
	if err != nil {
		t.Fatalf("stdout 있는데 err=%v", err)
	}
	if !strings.Contains(out, "data") {
		t.Errorf("stdout 유실: %q", out)
	}
}

// Strict 모드(삭제/업데이트 실행)는 stdout이 있어도 비허용 종료코드를 실패로 본다.
func TestRunStrictFailsDespiteStdout(t *testing.T) {
	rec := &Recorder{}
	r := NewRunner(rec, 10)
	_, err := r.Run(context.Background(), Opt{Strict: true}, "sh", "-c", "echo partial-output; exit 1")
	if err == nil {
		t.Fatal("Strict인데 부분 실패가 성공으로 처리됨")
	}
	// stderr가 비어도 stdout에서 원인을 확보해야 한다.
	if !strings.Contains(err.Error(), "partial-output") {
		t.Errorf("err에 원인 없음: %v", err)
	}
}

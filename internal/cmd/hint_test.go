package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestDirOnPATH(t *testing.T) {
	sep := string(filepath.ListSeparator)
	pathEnv := "/usr/bin" + sep + "/Users/c/go/bin" + sep + "/opt/homebrew/bin/"
	cases := []struct {
		dir  string
		want bool
	}{
		{"/Users/c/go/bin", true},
		{"/usr/bin", true},
		{"/opt/homebrew/bin", true},  // PATH 항목의 끝 슬래시는 무시
		{"/opt/homebrew/bin/", true}, // 인자 쪽 끝 슬래시도
		// go run이 만드는 임시 디렉토리 — 여기가 힌트를 띄워야 하는 상황이다.
		{"/var/folders/xx/T/go-build123/b001/exe", false},
		{"/Users/c/Documents/develop/macinv", false},
		{"", false},
	}
	for _, c := range cases {
		if got := DirOnPATH(c.dir, pathEnv); got != c.want {
			t.Errorf("DirOnPATH(%q) = %v, want %v", c.dir, got, c.want)
		}
	}
	// 빈 PATH 항목("")은 현재 디렉토리를 뜻하지만 무시한다 — 오판하면 힌트가 영영 안 뜬다.
	if DirOnPATH("/anything", sep+sep) {
		t.Error("빈 PATH 항목이 매칭됨")
	}
}

func TestShouldHintInstall(t *testing.T) {
	withFlags := func(name string, json, quiet bool) *cobra.Command {
		c := &cobra.Command{Use: name}
		var j, q bool
		c.Flags().BoolVar(&j, "json", json, "")
		c.Flags().BoolVar(&q, "quiet", quiet, "")
		if json {
			_ = c.Flags().Set("json", "true")
		}
		if quiet {
			_ = c.Flags().Set("quiet", "true")
		}
		return c
	}

	cases := []struct {
		name   string
		cmd    *cobra.Command
		onPATH bool
		want   bool
	}{
		{"PATH 밖 일반 실행 → 안내", withFlags("scan", false, false), false, true},
		{"이미 PATH에 있음 → 침묵", withFlags("scan", false, false), true, false},
		{"--json → 침묵(파이프 오염 방지)", withFlags("scan", true, false), false, false},
		{"--quiet → 침묵", withFlags("scan", false, true), false, false},
		{"TUI → 침묵(대체 화면)", withFlags("tui", false, false), false, false},
		{"플래그 없는 커맨드도 안내", &cobra.Command{Use: "trace"}, false, true},
		{"nil 커맨드", nil, false, false},
	}
	for _, c := range cases {
		if got := shouldHintInstall(c.cmd, c.onPATH); got != c.want {
			t.Errorf("%s: = %v, want %v", c.name, got, c.want)
		}
	}
}

// 안내 문구는 **실제로 동작하는** 명령이어야 한다.
// 이 저장소는 private이라 go install <module>@latest는 프록시가 못 읽어 실패한다.
func TestHintSuggestsWorkingCommand(t *testing.T) {
	if !contains(globalInstallHint, "go install .") {
		t.Errorf("안내에 동작하는 명령이 없음: %q", globalInstallHint)
	}
	if !contains(globalInstallHint, "private") {
		t.Errorf("@latest가 왜 안 되는지 설명이 없음: %q", globalInstallHint)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

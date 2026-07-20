package source

import (
	"reflect"
	"testing"
)

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"v22.22.2\n":                                    "22.22.2", // node
		"go version go1.26.5 darwin/arm64":              "1.26.5",  // go
		"rustc 1.75.0 (82e1608df 2023-12-21)":           "1.75.0",  // rust
		"openjdk version \"21.0.1\" 2023-10-17":         "21.0.1",  // java (stderr)
		"Python 3.13.1":                                 "3.13.1",  // python
		"Apple Swift version 6.0 (swift-6.0-RELEASE)":   "6.0",     // swift (2 components)
		"PHP 8.3.0 (cli) (built: ...)":                  "8.3.0",   // php
		"":                                              "",        // 미설치/빈 출력
	}
	for in, want := range cases {
		if got := extractVersion(in); got != want {
			t.Errorf("extractVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractVersionSwift(t *testing.T) {
	// swift는 드라이버 버전(1.148.6)이 먼저 나오므로 전용 정규식으로 Apple Swift 버전을 잡아야 한다.
	out := "swift-driver version: 1.148.6 Apple Swift version 6.0 (swift-6.0-RELEASE)\nTarget: arm64-apple-macosx15.0\n"
	if got := extractVersionRe(out, swiftRe); got != "6.0" {
		t.Errorf("swift 버전 = %q, want 6.0 (드라이버 버전 오인)", got)
	}
	// 일반 추출이었다면 드라이버 버전을 잘못 잡음을 확인(대비용).
	if got := extractVersion(out); got != "1.148.6" {
		t.Errorf("일반 추출 = %q (드라이버 버전이 먼저라 6.0이 아님이 정상)", got)
	}
}

func TestClassifyManaged(t *testing.T) {
	env := TraceEnv{GoBin: "/Users/cris/go/bin", Home: "/Users/cris"}
	cases := []struct {
		resolved string
		want     string
	}{
		{"/Users/cris/.nvm/versions/node/v22.22.2/bin/node", MgrNvm},
		{"/Users/cris/.pyenv/versions/3.12.0/bin/python", MgrPyenv},
		{"/Users/cris/.rustup/toolchains/stable-aarch64-apple-darwin/bin/rustc", MgrRustup},
		{"/Users/cris/.asdf/installs/ruby/3.3.0/bin/ruby", MgrAsdf},
		{"/Users/cris/.sdkman/candidates/java/21-open/bin/java", MgrSdkman},
		{"/Users/cris/.local/share/mise/installs/node/20.11.0/bin/node", MgrMise},
		// 버전 매니저가 아니면 기존 Classify로 폴백.
		{"/opt/homebrew/Cellar/go/1.26.5/libexec/bin/go", MgrBrew},
		{"/usr/bin/python3", MgrSystem},
	}
	for _, c := range cases {
		if got, _ := ClassifyManaged(c.resolved, env); got != c.want {
			t.Errorf("ClassifyManaged(%q) = %q, want %q", c.resolved, got, c.want)
		}
	}
}

func TestVersionsFromMatches(t *testing.T) {
	// 1단계(nvm 등): 마지막 조각이 버전.
	got := versionsFromMatches([]string{
		"/Users/cris/.nvm/versions/node/v20.11.0",
		"/Users/cris/.nvm/versions/node/v22.22.2",
	}, false)
	if want := []string{"v20.11.0", "v22.22.2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("1단계 = %v, want %v", got, want)
	}
	// 2단계(asdf 등): "<tool> <version>", current/숨김은 제외.
	got = versionsFromMatches([]string{
		"/Users/cris/.asdf/installs/ruby/3.3.0",
		"/Users/cris/.sdkman/candidates/java/current",
	}, true)
	if want := []string{"ruby 3.3.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("2단계 = %v, want %v", got, want)
	}
}

package source

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

// goBinSource는 $GOBIN(또는 ~/go/bin)의 실행 파일을 스캔한다. 외부 명령을 쓰지 않는다.
type goBinSource struct{}

func (s *goBinSource) Name() string { return NameGoBin }

func goBinDir() string {
	if v := os.Getenv("GOBIN"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "go", "bin")
}

func (s *goBinSource) Available() bool {
	fi, err := os.Stat(goBinDir())
	return err == nil && fi.IsDir()
}

func (s *goBinSource) Scan(_ context.Context) (any, error) {
	dir := goBinDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []report.GoBinItem{}, nil
	}
	out := []report.GoBinItem{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.Mode()&0o111 != 0 { // 실행 권한 있음
			out = append(out, report.GoBinItem{
				Name: e.Name(),
				Path: filepath.Join(dir, e.Name()),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

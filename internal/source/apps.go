package source

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/report"
)

// appsSource는 /Applications 등지의 *.app을 스캔한다.
type appsSource struct {
	run      *iexec.Runner
	withSize bool
}

func (s *appsSource) Name() string    { return NameApps }
func (s *appsSource) Available() bool  { return true } // 앱 디렉토리는 항상 존재

func appRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/Applications",
		filepath.Join(home, "Applications"),
		"/Applications/Utilities",
	}
}

func (s *appsSource) Scan(ctx context.Context) (any, error) {
	var apps []string
	for _, root := range appRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			if !os.IsNotExist(err) {
				s.run.Rec.Add("권한 없음 또는 읽기 실패: " + root)
			}
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".app") {
				apps = append(apps, filepath.Join(root, e.Name()))
			}
		}
	}

	results := make([]report.App, len(apps))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // worker pool 8
	for i, path := range apps {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = s.scanOne(ctx, path)
		}(i, path)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})
	return results, nil
}

// mdls 속성명. 순서에 의존하지 않도록 -name 하나씩 개별 조회한다
// (프로토타입은 여러 -name을 한 번에 넘겨 반환 순서가 뒤바뀌는 버그가 있었다).
const (
	attrBundleID  = "kMDItemCFBundleIdentifier"
	attrVersion   = "kMDItemVersion"
	attrDateAdded = "kMDItemDateAdded"
)

func (s *appsSource) scanOne(ctx context.Context, path string) report.App {
	app := report.App{
		Name: strings.TrimSuffix(filepath.Base(path), ".app"),
		Path: path,
	}
	app.BundleID = s.mdlsOne(ctx, path, attrBundleID)
	app.Version = s.mdlsOne(ctx, path, attrVersion)
	app.DateAdded = s.mdlsOne(ctx, path, attrDateAdded)
	if app.DateAdded == nil {
		app.DateAdded = birthtime(path)
	}
	if s.withSize {
		app.SizeKB = s.duKB(ctx, path)
	}
	return app
}

// mdlsOne은 속성 하나를 -raw로 조회한다. (null)/빈 값이면 nil.
func (s *appsSource) mdlsOne(ctx context.Context, path, attr string) *string {
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 20}, "mdls", "-raw", "-name", attr, path)
	if err != nil {
		return nil
	}
	v := strings.TrimSpace(out)
	if v == "" || v == "(null)" {
		return nil
	}
	return &v
}

// duKB는 du -sk로 KB 용량을 구한다.
func (s *appsSource) duKB(ctx context.Context, path string) *int {
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 60}, "du", "-sk", path)
	if err != nil {
		return nil
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return nil
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil
	}
	return &n
}

// birthtime은 Spotlight 메타데이터가 없을 때 파일 생성시각으로 폴백한다.
func birthtime(path string) *string {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	// darwin에서 ModTime을 폴백으로 사용(생성시각 syscall 접근은 플랫폼 의존이 커서 단순화).
	s := fi.ModTime().Format("2006-01-02 15:04:05")
	return &s
}

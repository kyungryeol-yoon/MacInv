package source

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
	"github.com/kyungryeol-yoon/MacInv/internal/paths"
	"github.com/kyungryeol-yoon/MacInv/internal/report"
)

// appsSource는 /Applications 등지의 *.app을 스캔한다.
type appsSource struct {
	run      *iexec.Runner
	withSize bool
	progress func(stage string, done, total int) // nil 가능
}

func (s *appsSource) Name() string    { return NameApps }
func (s *appsSource) Available() bool { return true } // 앱 디렉토리는 항상 존재

func (s *appsSource) Scan(ctx context.Context) (any, error) {
	var apps []string
	for _, root := range paths.AppRoots() {
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
	// 데이터 용량 측정(du)은 앱당 수 초까지 걸려 진행 표시가 없으면 멈춘 것처럼 보인다.
	// 완료 카운터는 여러 goroutine이 올리므로 뮤텍스로 직렬화한다.
	var (
		progMu sync.Mutex
		done   int
	)
	reportProgress := func() {
		if s.progress == nil {
			return
		}
		progMu.Lock()
		done++
		n := done
		progMu.Unlock()
		s.progress("앱 용량 측정 중", n, len(apps))
	}
	for i, path := range apps {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = s.scanOne(ctx, path)
			if s.withSize {
				reportProgress()
			}
		}(i, path)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})
	return results, nil
}

// mdls 속성명.
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
	// mdls를 앱당 한 번만 호출한다(3회 → 1회, 프로세스 스폰 비용 절감).
	// -raw의 위치 기반 출력은 순서가 뒤바뀌는 버그(프로토타입)가 있어,
	// "키 = 값" 형식을 키 기반으로 파싱해 순서 문제 자체를 없앤다.
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 20}, "mdls",
		"-name", attrBundleID, "-name", attrVersion, "-name", attrDateAdded, path)
	if err == nil {
		attrs := parseMdlsKeyed(out)
		app.BundleID = report.PtrOrNil(attrs[attrBundleID])
		app.Version = report.PtrOrNil(attrs[attrVersion])
		app.DateAdded = report.PtrOrNil(attrs[attrDateAdded])
	}
	if app.BundleID == nil {
		// mdls가 (null)을 주는 앱이 있다 — Info.plist 직독으로 폴백.
		// Bundle ID가 없으면 clean/용량 집계가 이름 기반으로만 돌아 잔여물을 놓친다.
		app.BundleID = report.PtrOrNil(paths.BundleIDFromPlist(ctx, s.run, path))
	}
	if app.DateAdded == nil {
		app.DateAdded = birthtime(path)
	}
	if s.withSize {
		app.SizeKB = s.duKB(ctx, path)
		app.DataSizeKB = s.dataKB(ctx, app)
	}
	return app
}

// dataKB는 앱 번들 밖(~/Library 등)에 쌓인 데이터/캐시 용량을 잰다.
// 경로 목록은 paths의 측정용 규칙을 쓴다(clean의 삭제 계획과 같은 저수준 매칭 위에 있다).
func (s *appsSource) dataKB(ctx context.Context, app report.App) *int {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	bundleID := ""
	if app.BundleID != nil {
		bundleID = *app.BundleID
	}
	matches := paths.SizePaths(home, bundleID, app.Name)
	if len(matches) == 0 {
		return nil
	}
	// du를 경로마다 부르지 않고 한 번에 넘긴다(프로세스 스폰 1회).
	args := make([]string, 0, len(matches)+2)
	args = append(args, "-sk")
	for _, m := range matches {
		args = append(args, m.Path)
	}
	// 캐시가 수십 GB로 불어난 앱(Electron Service Worker 등)은 walk가 오래 걸린다.
	out, err := s.run.Run(ctx, iexec.Opt{TimeoutSec: 300}, "du", args...)
	if err != nil {
		return nil
	}
	return report.Ptr(sumDuKB(out))
}

// sumDuKB는 `du -sk a b c`의 여러 줄 출력에서 KB 합계를 낸다(순수 함수 — 테스트 대상).
// 권한 오류로 일부 경로가 빠져도 나머지는 그대로 합산한다.
func sumDuKB(out string) int {
	total := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if n, err := strconv.Atoi(fields[0]); err == nil {
			total += n
		}
	}
	return total
}

// parseMdlsKeyed는 mdls의 "속성 = 값" 줄들을 파싱한다(순수 함수 — 테스트 대상).
// (null)/빈 값은 제외하고, 문자열 값의 둘러싼 따옴표는 벗긴다.
func parseMdlsKeyed(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" || val == "(null)" {
			continue
		}
		m[key] = strings.Trim(val, `"`)
	}
	return m
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

package disk

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	iexec "github.com/kyungryeol-yoon/macinv/internal/exec"
	"github.com/kyungryeol-yoon/macinv/internal/paths"
)

// --deep은 "알려진 위치" 밖을 뒤진다. 기본 스캔이 3초 안에 끝나야 하므로 옵트인이다.
// 지금 찾는 것: 오래된 node_modules — 프로젝트를 그만 건드린 뒤에도 남아 수 GB를 먹는 전형적 케이스.

const (
	deepMaxDepth   = 6   // 홈 기준 탐색 깊이 상한(너무 깊이 들어가면 느려진다)
	deepMinAgeDays = 180 // 6개월 이상 손대지 않은 것만
	deepMaxResults = 50  // 출력 상한
	deepTimeoutSec = 120 // 전체 탐색 예산
	deepTarget     = "node_modules"
)

// DeepFinding은 --deep으로 찾은 항목 하나다.
type DeepFinding struct {
	Path    string `json:"path"`
	SizeKB  int    `json:"size_kb"`
	ModTime string `json:"mod_time"`
	AgeDays int    `json:"age_days"`
}

// deepSkipDirs는 탐색에서 통째로 건너뛸 디렉토리 이름이다.
// Library는 이미 알려진 위치가 담당하고, 미디어 라이브러리는 walk가 비싼데 얻을 게 없다.
// 클라우드 동기화 폴더는 walk가 원격 파일 내려받기를 유발할 수 있어 들어가지 않는다.
var deepSkipDirs = map[string]bool{
	"Library":      true,
	"Applications": true,
	"Pictures":     true,
	"Movies":       true,
	"Music":        true,
	"Google Drive": true,
	"Dropbox":      true,
	"OneDrive":     true,
}

// ScanDeep은 홈 아래에서 오래된 node_modules를 찾는다(읽기 전용).
// 시스템 경로·숨김 디렉토리·deepSkipDirs에는 아예 들어가지 않고, 깊이와 시간 예산을 건다.
func ScanDeep(ctx context.Context, run *iexec.Runner, home string, now time.Time) []DeepFinding {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(deepTimeoutSec)*time.Second)
	defer cancel()

	var found []string
	rootDepth := strings.Count(filepath.Clean(home), string(filepath.Separator))

	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 권한 없는 디렉토리는 조용히 건너뛴다
		}
		if cctx.Err() != nil {
			return filepath.SkipAll
		}
		if !d.IsDir() || p == home {
			return nil
		}
		if paths.IsProtected(p) {
			return filepath.SkipDir
		}
		name := d.Name()
		if name == deepTarget {
			found = append(found, p)
			return filepath.SkipDir // 중첩 node_modules 안까지 들어가지 않는다
		}
		if strings.HasPrefix(name, ".") || deepSkipDirs[name] {
			return filepath.SkipDir
		}
		if strings.Count(filepath.Clean(p), string(filepath.Separator))-rootDepth >= deepMaxDepth {
			return filepath.SkipDir
		}
		return nil
	})

	return measureDeep(cctx, run, found, now)
}

// measureDeep은 후보들의 나이/용량을 재서 조건에 맞는 것만 돌려준다.
func measureDeep(ctx context.Context, run *iexec.Runner, candidates []string, now time.Time) []DeepFinding {
	var out []DeepFinding
	var stale []string
	ages := map[string]int{}
	mods := map[string]string{}
	for _, p := range candidates {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		age := int(now.Sub(fi.ModTime()).Hours() / 24)
		if age < deepMinAgeDays {
			continue
		}
		stale = append(stale, p)
		ages[p] = age
		mods[p] = fi.ModTime().Format("2006-01-02")
	}
	if len(stale) == 0 {
		return nil
	}
	sizes := map[string]int{}
	if len(stale) > 0 {
		args := append([]string{"-sk"}, stale...)
		if o, err := run.Run(ctx, iexec.Opt{TimeoutSec: duTimeoutSec}, "du", args...); err == nil {
			sizes = parseDuLines(o)
		}
	}
	for _, p := range stale {
		out = append(out, DeepFinding{Path: p, SizeKB: sizes[p], ModTime: mods[p], AgeDays: ages[p]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SizeKB > out[j].SizeKB })
	if len(out) > deepMaxResults {
		out = out[:deepMaxResults]
	}
	return out
}

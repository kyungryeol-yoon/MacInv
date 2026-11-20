// Package source는 각 설치 소스(GUI 앱, brew, npm, ...)를 스캔하는 패키지다.
// 공통 Source 인터페이스로 추상화해 새 소스(gem, mise 등) 추가를 쉽게 하고,
// 스캔은 errgroup으로 병렬 실행한다. 스캔 로직은 여기, 출력은 report/cmd 레이어가 담당한다.
package source

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"

	iexec "github.com/kyungryeol-yoon/MacInv/internal/exec"
)

// 소스 이름 상수. ScanAll 결과 맵의 키이자 report 조립 시 참조된다.
const (
	NameApps    = "apps"
	NameBrew    = "brew"
	NameNpm     = "npm"
	NamePipx    = "pipx"
	NameCargo   = "cargo"
	NameMas     = "mas"
	NameGoBin   = "go_bin"
	NameLaunchd = "launchd"
)

// Source는 하나의 설치 소스를 스캔하는 인터페이스다.
type Source interface {
	// Name은 소스 식별자다(ScanAll 결과의 키).
	Name() string
	// Available은 이 소스를 스캔할 수 있는지(명령/디렉토리 존재) 여부다.
	// false면 스캔을 건너뛰고 결과는 nil(JSON null)로 둔다.
	Available() bool
	// Scan은 소스별 결과(report의 구체 타입)를 돌려준다.
	Scan(ctx context.Context) (any, error)
}

// Options는 스캔 동작 옵션이다.
type Options struct {
	WithSize bool // GUI 앱 용량(du) 계산 여부
	// Progress는 오래 걸리는 단계의 진행률 콜백이다(nil이면 보고하지 않음).
	// 앱 데이터 용량 측정(du)은 수십 GB 캐시를 walk해 수 초~수십 초가 걸려
	// 진행 표시가 없으면 멈춘 것처럼 보인다. 호출은 동시에 들어오므로 구현이 직렬화해야 한다.
	Progress func(stage string, done, total int)
}

// Default는 이 맥에서 스캔할 기본 소스 목록을 만든다.
func Default(run *iexec.Runner, opt Options) []Source {
	return []Source{
		&appsSource{run: run, withSize: opt.WithSize, progress: opt.Progress},
		&brewSource{run: run},
		&npmSource{run: run},
		&pipxSource{run: run},
		&cargoSource{run: run},
		&masSource{run: run},
		&goBinSource{},
		&launchdSource{run: run},
	}
}

// OutdatedSources는 outdated 체크에 필요한 소스(brew, npm, mas)만 돌려준다.
// brew/npm은 스캔 후 EnrichBrew/EnrichNpm으로 보강해야 하고, mas는 Scan 시점에 outdated를 함께 수집한다.
func OutdatedSources(run *iexec.Runner) []Source {
	return []Source{
		&brewSource{run: run},
		&npmSource{run: run},
		&masSource{run: run},
	}
}

// ScanAll은 사용 가능한 소스를 모두 병렬로 스캔해 이름→결과 맵을 돌려준다.
// 미설치 소스는 nil 값으로 담긴다. 개별 소스 에러는 경고로 처리되고 전체를 멈추지 않는다.
func ScanAll(ctx context.Context, srcs []Source, rec *iexec.Recorder) map[string]any {
	results := make(map[string]any, len(srcs))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	for _, s := range srcs {
		s := s
		g.Go(func() error {
			var val any
			if s.Available() {
				v, err := s.Scan(gctx)
				if err != nil {
					rec.Add(s.Name() + ": " + err.Error())
				} else {
					val = v
				}
			}
			mu.Lock()
			results[s.Name()] = val
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // 개별 소스는 에러를 반환하지 않으므로 항상 nil
	return results
}

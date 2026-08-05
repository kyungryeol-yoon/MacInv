# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트

`macinv` — macOS에 설치된 소프트웨어 전체를 스캔해 "내 맥에 뭐가 어디 깔려있는지" 파악하고, 업데이트 대기를 표시하고, 최종적으로 잔여물 없이 안전하게 삭제하는 Go CLI. 검증된 Python 프로토타입 `mac_inventory.py`(참조용으로 보존)를 Go로 재설계한 것이다. 사용자 대상 출력은 **한국어**(이모지 배너 포함).

- Go 모듈: `github.com/kyungryeol-yoon/macinv`
- **GitHub**: https://github.com/kyungryeol-yoon/macinv (**private**, `main` 브랜치). 다른 기기에서 이어서 작업하려면 `gh auth login` 또는 SSH 키로 clone/push 권한이 필요하다 — 단계는 [`README.md`의 설치 섹션](./README.md#설치) 참고. private이라 `go install github.com/...@latest`는 이 저장소에서 동작하지 않는다(clone 후 로컬 빌드/설치).
- Go 1.25+ (`go.mod`의 `go` 지시자 기준; 개발 환경은 brew의 go 1.26.5). 외부 의존성: `spf13/cobra`(CLI), `golang.org/x/sync/errgroup`(병렬 스캔), `charmbracelet/{bubbletea,bubbles,lipgloss}`(TUI) — 나머지는 이들의 전이 의존성(자세한 목록은 `go.mod`).

## 명령어

```bash
go build ./...            # 빌드
go vet ./...              # 정적 검사
go test ./...             # 전체 테스트
go test ./internal/source -run TestParseMas   # 단일 테스트
go run . scan             # 전체 스캔 + 요약 (실제 실행)
go install .              # ~/go/bin/macinv 로 설치
```

`macinv --version`(`-v`)으로 설치된 버전 확인(현재 0.1.0).

`macinv runtimes` 플래그: `--json`, `--offline`(EOL 조회 생략 — 캐시만 사용).

`macinv disk` 플래그: `--json`, `--apply`(정리 실행 — 확인 필요), `--include-expensive`(모델·이미지 포함), `--include-running`(실행 중인 앱의 캐시도 포함 — 오작동 위험), `--deep`(오래된 node_modules 탐색), `--quiet`.

`macinv scan` 플래그: `--json`(JSON을 stdout으로), `--no-outdated`(네트워크 최신버전 체크 생략), `--no-sizes`(du 용량 계산 생략 — 앱 번들+데이터 둘 다), `--no-runtimes`(언어 런타임 조회 생략), `--quiet`(요약 생략), `-o PATH`(저장 경로). 결과는 항상 `~/.macinv/inventory.json`에 캐시된다. scan은 설치된 언어 런타임 버전도 요약에 포함한다.

## 아키텍처

레이어로 스캔/데이터/출력을 분리한다 (TUI를 bubbletea로 얹기 위한 구조). **의존은 항상 아래로만** 흐른다:

```
cmd → tui → disk → clean ┐
                         ├→ paths → exec
              source ────┘     report(순수 데이터)
```

`go list -deps`로 검증 가능하다. 마일스톤 6-3 이전에는 `source/apps.go`가 `clean`을 import해
방향이 뒤집혀 있었고(저수준 스캐너 → 고수준 플래너), 경로 매칭 유틸을 `internal/paths`로
빼면서 해소했다. **경로 판별·매칭 규칙은 전부 paths에 한 벌만 둔다** — source(측정)와
clean(삭제 계획)이 서로 다른 규칙을 갖게 되는 순간 "보이는 용량"과 "지워지는 것"이 어긋난다.

- **`internal/paths/`** — 경로 매칭·판별 유틸(최하층, exec 외에는 아무것도 import하지 않는다).
  시스템 경로 블랙리스트(`IsProtected`/`IsAppleSystemApp`), 잔여물 매칭(`LeftoverPaths` ←
  `bundleGlobs`/`nameGlobs`/`gatherLeftovers`), 측정 전용 경로(`SizePaths`, `vendorAppDirs`,
  `groupContainersByName`), 벤더 테이블(`data/vendors.json` `go:embed` + `Vendors`/`LookupVendor`/
  `CountVendorApps`), Bundle ID 유틸(`BundleIDFromPlist`/`DerivedBundleID`/`HasTeamIDPrefix`/
  `StripHostUUID`), 중복 경로 제거(`DedupContained`). clean은 `PathMatch`/`ConfHigh` 등을
  **타입 별칭·변수 별칭으로 재노출**해 기존 공개 API(`clean.PathMatch`, `clean.IsProtected` …)를 유지한다.
- **`internal/source/`** — 스캔 로직. 각 소스(apps/brew/npm/pipx/cargo/mas/gobin/launchd)가 `Source` 인터페이스(`Name() / Available() / Scan(ctx)`)를 구현한다. `source.Default()`가 기본 소스 목록을, `source.ScanAll()`이 `errgroup`으로 **병렬 스캔**해 이름→결과 맵을 돌려준다. 새 소스(gem, mise 등) 추가 = 파일 하나 + `Default()`에 한 줄.
  - **파싱은 exec와 분리**되어 있다. `Scan`은 명령을 실행하고, 순수 파서 함수(`parseBrewInfo`, `parseNpmLs`, `parseMas`, `parseCargoList`, `parseLaunchdJSON`, `applyBrewOutdated`, `applyNpmOutdated` 등)가 바이트/문자열을 받아 report 타입을 돌려준다. 테스트는 이 파서들을 fixture(`internal/source/testdata/`)로 직접 검증한다.
  - `Available()`이 false(명령 미설치 / 디렉토리 없음)면 스캔을 건너뛰고 결과는 nil → JSON `null`. **미설치는 에러가 아니라 조용한 스킵**이다.
- **`internal/exec/`** — 외부 명령 공통 실행기 `Runner`. 모든 호출에 `context.WithTimeout` 적용. 실패해도 panic하지 않고 `Recorder`에 경고를 모아 리포트 `errors` 필드로 내보낸다. 종료코드가 허용 밖이어도 **stdout이 있으면 성공**으로 본다(프로토타입 규칙, 스캔/읽기 전용). `npm ls`/`npm outdated`처럼 정상 상황에 exit 1을 내는 명령은 `Opt.OKCodes: []int{0,1}`로 허용한다. 실행 파일 자체가 없으면 `ErrNotFound`(경고 없음). **변경 명령(clean/update의 `Execute`·`MoveToTrash`)은 `Opt.Strict`로 엄격 판정** — 부분 실패를 성공으로 오인하지 않는다. 실패 에러 메시지에는 stderr(없으면 stdout) 상세가 포함된다.
- **`internal/report/`** — JSON 스키마 타입(프로토타입 준수)과 한국어 요약 렌더(`PrintSummary`). null 가능 필드는 포인터(`*string`/`*int`/`*bool`)로 두어 JSON에서 `null`로 나간다. `Ptr`/`PtrOrNil` 헬퍼 사용. **런타임/소유권 데이터 타입(`Runtime`/`Origin`/`VersionManager`)도 여기 있다** — source가 report를 import하므로 순환 없이 source(로직)/tui/cmd가 공유한다(source는 `type Runtime = report.Runtime` 별칭으로 노출).
- **`internal/clean/`** — 삭제·업데이트·고아 잔여물 **플래너**(경로 매칭은 paths에 위임하고 여기선 계획 수립만). `BuildPlan`/`BuildUpgradePlan`/`BuildOrphansPlan`은 읽기 전용으로 계획만, `Execute`(휴지통 이동/명령 실행)는 cmd가 `--apply`+확인 후에만 호출. CLI/TUI가 공유하는 계획 렌더는 `clean.PlanLines`/`RenderPlanText`(`render.go`).
  - **명령은 `Target.Argv [][]string`**(문자열 아님). 표시용 한 줄은 렌더 시점에 `ShellQuote`가 만든다. 예전에는 `"brew uninstall foo"` 문자열로 저장하고 실행 직전 `strings.Fields`로 다시 쪼갰는데, 그러면 `--filter "until=24h"` 같이 **공백이 든 인자가 깨진다**(disk 커맨드에서 반드시 터질 형태). `ShellQuote`는 표시 전용이라 결과 문자열을 다시 파싱하는 코드는 없어야 한다.
  - **접두사 과매칭 방어**(`excludeOtherAppIDs`/`ownerOfID`, clean.go): `bundleGlobs`가 접두사 매칭이라 `com.google.Chrome` 계획에 `com.google.Chrome.canary`가 [확실]로 딸려 들어온다. 경로에서 뽑은 파생 ID(`paths.DerivedBundleID`)가 **설치된 다른 앱의 Bundle ID에 귀속**되면 계획에서 빼고 `Target.Excluded`에 "타 앱(<이름>) 소유 — 보존"을 남긴다. 귀속 판정은 **가장 긴 ID가 이긴다** — Chrome/Chrome.canary가 둘 다 설치된 상태에서 canary를 지울 때 canary 파생물이 Chrome 소유로 오판되면 안 된다. `com.google.Chrome.helper`처럼 **어느 앱도 소유하지 않는 파생물은 그대로 남긴다**(이 둘을 구분하는 게 핵심). 비싼 `installedAppList`(앱마다 mdls)는 `sync.OnceValue` + "검사할 파생 ID가 있을 때만" 게이트로 호출한다.
  - **권한 사전 확인**(`permission.go`): `bundleGlobs`는 `/Library/...`도 [확실]로 잡지만 대부분 root 소유라 휴지통 이동이 실패하고 로그에 ✗만 남는다. 계획 단계에서 **부모 디렉토리 쓰기 권한**(`syscall.Access`)을 보고 `Target.NeedsAdmin`으로 분리해 "🔑 권한 필요(수동)"로 표시하고 TrashPaths·회수 예상에서 뺀다. 실기 확인: `logioptionsplus`의 `/Library/LaunchDaemons/...`, `LogiPluginService.app`(부모 `/Applications/Utilities`가 root:wheel).
  - **회수 예상**(`reclaim.go`): `MeasureTrashSizes`가 계획에 담긴 경로만 `du -sk` 한 번으로 재고(`DedupContained`로 부모/자식 합쳐 이중 계산 방지 — 부모에만 값이 실린다), `ReclaimLine`이 하단 요약 줄을 만든다. 앱 목록의 표시 용량과 **다른 값인 게 정상**이다(아래 6-5 참고).
- **`internal/disk/`** — "왜 디스크가 꽉 찼는지"를 답하는 레이어(마일스톤 7). 알려진 캐시/데이터 위치는 `data/locations.json`에 **데이터로** 둔다(vendors.json과 같은 `go:embed` 방식) — 새 위치 추가에 코드 수정이 필요 없다. 측정은 `Scan`(errgroup, 위치당 `du -sk` 1회, 개별 타임아웃 120초, worker 6), 판정은 순수 함수(`Cleanable`/`IsOrphanCache`/`ResolvePaths`/`HasAppleComponent`/`GroupUsage`/`BreakdownLine`/`ParseOllamaList`/`ParseSnapshots`/`ParseDF`). **정리 실행은 새로 만들지 않고 `clean.Plan`/`Execute`/`RenderPlanText`를 재사용한다**(`BuildCleanupPlan`) — 안전 규칙이 두 벌이 되면 안 된다.
  - **`Entry.PathSizes`(경로별 용량)는 합계와 별개로 반드시 채운다.** ① 휴지통 계획은 경로마다 **제 용량**을 실어야 한다 — 위치 합계를 경로마다 넣으면 경로가 N개일 때 회수 예상이 N배가 된다(DerivedData는 경로 1개라 안 드러났고, Electron 캐시 9개 경로에서 1 GB→9 GB로 드러났다). ② 와일드카드 위치의 앱별 내역을 낼 수 없다.
  - **와일드카드 위치는 `GroupBy`로 쪼개 보여준다**(`"app-support-dir"`). 한 항목에 66 GB라고만 뜨면 **어느 앱을 손봐야 할지 판단이 안 된다**. `GroupUsage`가 앱별로 묶고 `BreakdownLine`이 "Claude 934 MB · Code 69 MB" 한 줄을 만든다(상위 5개, 나머지는 "외 N개"). CLI 요약·TUI 디스크 탭·`--json`(`breakdown`)이 공유.
  - **`HasAppleComponent`**: `paths.IsProtected`는 경로 **접두사** 블랙리스트라 `~/Library` 아래의 애플 데이터를 못 거른다. 와일드카드가 `com.apple.*` 디렉토리를 삼키지 않게 경로 컴포넌트 단위로 한 번 더 본다.
- **`internal/tui/`** — bubbletea 대화형 뷰(`tui.go` 상태/흐름, `view.go` 렌더/탭). 탭: 앱/brew/**패키지(npm·pipx·cargo·go)**/런타임/업데이트/**디스크**/launchd.
  - **앱 탭 정렬 토글**: `s`로 이름순↔용량순(번들+데이터 합계) 전환. 상태는 `Model.sortBySize`, 정렬은 순수 함수 `sortAppsBySize`(원본 `rep.Sections.Apps`를 건드리지 않고 **사본**을 정렬 — 다른 탭/커맨드가 공유한다). 용량 미측정 앱은 뒤로 보내고 그들끼리는 이름순 유지. 토글 시 `buildTabs` 재호출 + `sel/top` 초기화(행 순서가 통째로 바뀌므로). `marked`는 행 인덱스가 아니라 **target 이름 기준**이라 정렬해도 선택이 유지된다. 푸터 힌트는 용량을 표시하는 앱 탭에서만 노출.
  - **`enter`는 browse 모드에서 아무 일도 하지 않는다**(삭제는 `d`, 업데이트는 `u`만). 계획 모달의 승인도 enter라, 목록에서 enter가 삭제를 시작하면 brew/npm 항목(y/N 재확인 없음)은 **enter 두 번으로 실제 삭제**된다. 테스트로 고정(`TestEnterDoesNotStartDeleteInBrowse`).
  - **실행 가능**: `space` 멀티선택 → `d`(삭제) 또는 `u`(업데이트) → **계획 모달**(CLI와 **동일한 `clean.PlanLines`/`RenderPlanText` 공유**) → `[확신낮음]`은 체크박스 기본 해제(`space` 토글) → GUI 앱은 `y/N` 추가 확인, brew/npm 등은 승인 즉시 → `clean.Execute`를 goroutine에서 실행, 각 줄을 `chan tea.Msg`→`waitEvent` 루프로 **하단 로그 패널에 실시간 스트리밍**(UI 비블로킹), **동시 1개만 큐잉**. 완료 후 `RescanFunc`(cmd가 `RescanSources`로 주입)로 **영향 소스만 부분 재스캔**해 목록 갱신 + `PersistFunc`(cmd의 `PersistReport`)로 `~/.macinv/inventory.json` 캐시도 동기화. `r`=전체 재스캔, `esc`=대기 큐 취소(실행 중 외부 명령은 강제 종료 못 함). 잡 완료 시 실패 건수 요약(`jobDoneMsg.failed`)을 로그에 남긴다. 안전 규칙은 전부 `clean` 재사용. 모드(browse/building/plan/confirm)와 계획 생성/실행은 `tea.Cmd`로 비동기. TTY 없이 모델 전이·네비게이션·삭제/업데이트 프롬프트를 테스트한다.
  - **디스크 탭**은 다른 탭과 대상 종류가 달라(앱/패키지 이름이 아니라 캐시 위치 ID) `row.target`에 `disk:` 접두사를 붙여 네임스페이스를 나눈다. `selectedTargets`는 이걸 걸러내고 `selectedDiskIDs`가 따로 모은다 — 섞이면 앱 삭제 계획에 캐시 ID가 들어간다. `d`는 디스크 탭에서만 `disk.BuildCleanupPlan`을 쓰고(계획/실행/렌더는 clean 공용), `u`는 무시한다(캐시는 업데이트 대상이 아니다). 잡 완료 후 재스캔은 인벤토리 소스가 아니라 센티널 `sourceDisk`로 신호해 `DiskScanFunc`를 돌린다(`splitDiskSentinel`).
  - **TUI에서 expensive 포함 규칙**: CLI는 `--include-expensive` 플래그지만 TUI는 "그 행을 직접 골라 `d`를 눌렀다"는 사실 자체가 옵트인이다. 대신 계획에 expensive가 있으면 `clean.Plan.RequireConfirm`으로 GUI 앱과 같은 y/N 재확인을 받는다.
  - **탭 라벨 카운트**(`tab.count`)는 `len(tab.rows)`와 **다르다** — rows에는 "(없음)"/"✅ 최신 상태" 같은 안내 플레이스홀더 행이 섞여 있어, 헤더 숫자는 `buildTabs`에서 실제 데이터 길이(`brewCount`/`packageCount`/`installedRuntimeCount`/`len(outdated)` 등)로 **별도 계산**한다. 스크롤/선택 clamp는 여전히 `len(rows)` 사용(플레이스홀더도 화면엔 한 줄 차지하므로). 새 탭 추가 시 이 둘을 혼동하지 말 것 — 실제로 업데이트가 0개인데 "업데이트 (1)"로 뜨던 버그가 이 혼동에서 나왔다.
- **`internal/cmd/`** — cobra 커맨드(출력 레이어). **전역 설치 힌트**(`hint.go`): 실행 중인 바이너리가 PATH 밖이면(= `go run .`이나 프로젝트 경로 직접 실행) 종료 시 stderr에 설치 방법을 한 줄 안내한다. 억제 조건은 순수 함수 `shouldHintInstall`에 모았다 — 이미 PATH에 있음 · TUI(대체 화면) · `--json`(파이프 오염) · `--quiet`. **안내 명령은 `go install .`이다** — 이 저장소는 private이라 `go install <module>@latest`가 공개 모듈 프록시에서 실패한다. 힌트가 뜨는 상황은 애초에 저장소 안이라 `go install .`이 바로 먹힌다(public 전환 시 문구만 바꾸면 된다). 진행률은 `progressPrinter`(`scan.go`)가 stderr에 `\r`로 한 줄을 덮어써 갱신한다(`source.Options.Progress` → `appsSource`가 앱 하나 끝날 때마다 호출). stdout이 아니라 stderr인 이유는 `--json` 파이프를 오염시키지 않기 위해서고, `--quiet`면 콜백 자체가 nil, stderr가 TTY가 아니면 완료 줄만 남긴다. 스캔 수집은 `collectInventory`(scan/tui 공용)로 모았다: `buildSections` 조립 + `--no-outdated`가 아니면 `EnrichBrew`/`EnrichNpm` 보강 + `--no-runtimes`가 아니면 `ScanRuntimes`/`ScanVersionManagers`. 결과는 `report.Report`(runtimes/version_managers는 `omitempty`). outdated 수집은 `collectOutdatedRows`로 `outdated`/`update --all`이 공유.

### 알려진 주의점
- **mdls swap 버그**: 프로토타입은 `mdls -raw`에 여러 `-name`을 한 번에 넘겨 반환 순서가 뒤바뀌어 앱의 `version`/`date_added`가 서로 바뀌어 저장됐다. Go판은 mdls를 앱당 1회만 호출하고 **"키 = 값" 형식을 키 기반으로 파싱**(`parseMdlsKeyed`)해 이 버그와 구조적으로 무관하게 만들었다. 프로토타입 JSON과 diff 시 이 두 필드 값이 다른 것은 정상.
- JSON 스키마는 프로토타입과 **키 단위로 완전 일치**했으나(M1 검증), 이후 사용자 요청으로 `runtimes`/`version_managers` 최상위 필드를 **의도적으로 추가**했다(`omitempty`라 outdated/trace 등 다른 커맨드 출력엔 안 나옴).

## 마일스톤

진행은 마일스톤 단위. 각 단계마다 이 맥에서 실제 실행 → 결과 확인 → 다음. 1~7 완료.

1. **scan** ✅ — 전체 스캔 + 요약 + JSON 캐시.
2. **outdated** ✅ — brew/npm/mas 업데이트 대기 전용 뷰 (`--json`). `source.OutdatedSources`로 3개 소스만 스캔 후 보강, `report.OutdatedRows`로 행 집계(요약과 공용), `text/tabwriter`로 표 출력.
3. **trace `<name>`** ✅ — `which -a` 각 경로를 `filepath.EvalSymlinks`로 해소해 소유권 판별. 순수 함수 `source.Classify`(경로만 보고 brew Cellar/Caskroom · npm node_modules · pipx venvs · cargo · go bin · system · manual 판별)로 분리 → runtimes에서 재사용. PATH 첫 경로를 Active(★)로 표시. `--json` 지원.
4. **runtimes** ✅ — node/python/go/rust/java/ruby/php/dotnet/swift의 활성 버전+경로+관리주체를 병렬 조회. 버전은 `exec.RunCombined`(stdout+stderr 합침, java/swift 대응)로 얻어 `extractVersion`(순수 함수)으로 추출, swift는 `swift-driver` 버전 오인을 막는 전용 정규식 사용. 관리 주체는 `ClassifyManaged`(nvm/pyenv/mise/asdf/rustup/sdkman 경로 우선 → `Classify` 폴백, `TraceManaged` 경유). 서로 다른 실제 경로가 2개 이상이면 `Conflict`로 ⚠️ 표시. 버전 매니저는 `ScanVersionManagers`가 홈 디렉토리 glob(`~/.nvm/versions/node/*` 등)만으로 관리 버전을 열거(외부 명령 없음). `--json` 지원.
4-1. **runtimes EOL/최신버전**(2026-08-03) — 목적은 **최신 강요가 아니라 판단 근거 제공**이다. 몇 달 만에 어떤 스택으로 돌아왔을 때 "깔려 있나 · 몇 버전인가 · 올려야 하나"에 한 번에 답한다. 로직은 `internal/source/eol.go`, 상태 상수·배지 렌더는 `internal/report/runtimes.go`(report.Runtime의 필드를 해석하는 일이라 거기 둔다. source는 별칭으로 노출).
   - **조회**: endoflife.date `/api/<product>.json`. `java`/`openjdk` 슬러그는 **404라 없다** — OpenJDK 수명주기의 사실상 기준인 `eclipse-temurin`(Adoptium)을 쓴다. 대상은 python/nodejs/go/eclipse-temurin/ruby/php/rust(dotnet·swift는 제외).
   - **실패는 에러가 아니다**: 오프라인·조회 실패·캐시 없음이면 **조용히 생략**하고 설치 현황만 보여준다. `~/.macinv/eol-cache.json`에 7일 캐시, `--offline`로 조회 생략(캐시가 있으면 캐시 사용). `scan`/TUI는 **항상 캐시만** 쓴다(`EnrichEOL(..., offline=true, ...)`) — 자주 도는 경로에 네트워크 왕복을 얹지 않는다. 캐시는 `macinv runtimes`가 채운다.
   - **사이클 매칭은 가장 긴 접두사 일치**(`MatchCycle`): 사이클 자릿수가 제각각이라("3.9" python, "26" node/java, "1.26" go/rust) 짧은 쪽이 먼저 걸리면 오답이 된다("1.2"가 "1.26.5"를 집으면 안 됨).
   - **업데이트는 세 갈래**(`compareVersions`): `[현재]`/`[안전한 패치 있음]`/`[메이저 최신은 X]`. `LatestVersion`(내 사이클 안의 최신 패치)과 `LatestCycle`(나와 있는 최신 사이클)을 **따로 둔다** — 패치와 메이저는 위험도가 전혀 다르다.
   - **런타임은 자동 실행 대상이 아니다.** 권장 명령(`SuggestUpgradeCommand`)은 **보여주기만** 한다. TUI 런타임 행은 `target`이 비어 있어 `d`/`u`가 먹지 않는다(`TestRuntimeRowsAreNotActionable`로 고정). 시스템 런타임은 제자리 업그레이드가 안 되므로 "따로 설치"(mise가 있으면 mise, 없으면 brew install)를 권한다.
   - **`update --all`은 런타임 메이저를 실행하지 않는다**(`clean.SplitRuntimeMajors`). brew는 node 25→26 같은 점프도 그냥 outdated로 보고하는데, **`--all`은 "생각 없이 실행하는 명령"이라 프로젝트를 깨뜨릴 수 있는 것이 들어가면 안 된다.** 목록에는 `⏸ 제외했습니다` 섹션으로 보여주고 `$ brew upgrade node` 명령만 알려준다(사용자가 직접 실행). 패치(같은 사이클)는 그대로 포함한다. 개별 `update <name>`은 여전히 실행하되 계획에 경고가 붙는다 — 이름을 직접 적은 건 의도가 분명하기 때문이다.
   - **"메이저"의 의미는 언어마다 다르다**(`runtimeCycleParts`): node/openjdk/deno/bun는 1마디(25→26), python/ruby/php는 2마디(3.13→3.14).
     - **go/rust는 1.x 안에서 경고하지 않는다 — 이건 의도된 결정이니 뒤집지 말 것.** 두 언어 모두 1.x 안에서 하위 호환을 **정책으로** 보장한다(Go 1 호환성 약속, Rust 안정성 보장). 다른 언어의 "메이저"와 개념이 달라서, 1.26→1.27에 경고를 띄우면 릴리스마다 뜨는 **노이즈**가 되고 진짜 위험한 node 25→26 경고까지 같이 무시하게 만든다. 정말 깨지는 건 1→2 전환인데 그건 아직 없다.
     - `node@22`처럼 `@`가 붙은 고정 버전 포뮬러도 대상이 아니다(메이저가 이미 고정돼 있다).

5. **clean `<name>`** ✅ — 삭제 플래너. `internal/clean` 패키지. `BuildPlan`은 **읽기 전용**으로 계획만 만들고, 실제 삭제는 `Execute`(휴지통 이동)를 `--apply` + 대화형 확인(대상 이름 정확 입력) 후에만 호출.
   - **잔여물 탐지는 glob 접두사 매칭**(`bundleGlobs`/`nameGlobs`/`gatherLeftovers`)이라 헬퍼·파생 파일(`<bundleID>.ShipIt`, `<bundleID>.helper.plist` 등)과 Group Containers·Cookies·ByHost·시스템 `/Library`까지 잡는다.
   - **확신도 3단계**: `[확실]`(Bundle ID, `ConfHigh`) / `[확신중간]`(`ConfMedium`, 벤더 폴더 전체) / `[확신낮음]`(`ConfLow`, 이름 기반). `--apply`는 기본적으로 high+medium을 삭제하고 low는 `--include-low` 있을 때만(`Plan.WithoutLowConfidence`가 low만 제거).
   - **패키지 매니저 항목**(`packages.go`): query가 npm 전역/pipx/cargo 패키지거나 `~/go/bin` 실행 파일이면 네이티브 uninstall 명령(`npm uninstall -g`, `pipx uninstall`, `cargo uninstall`) 또는 바이너리 휴지통 이동을 타깃으로 추가.
   - **벤더 폴더 처리**(`vendor.go` + `data/vendors.json` `go:embed`): Google/Microsoft/Adobe/JetBrains 등 벤더 폴더를 만나면 **인벤토리(설치된 앱 Bundle ID)를 참조**해 같은 벤더 다른 앱이 있는지 본다. 있으면 `<Vendor>/<App>` 하위만 `[확신낮음]`으로 추가하고 공유 컴포넌트(Keystone 등)는 `Target.Excluded`에 "타 앱 사용 중" 사유로 보존 표기; 이 벤더 앱이 이것뿐이면 벤더 폴더 전체 + 고아 공유 컴포넌트를 `[확신중간]`으로. 새 벤더 케이스는 **코드 수정 없이 `data/vendors.json`에 추가**. `coveredByExisting`으로 부모 폴더에 중첩된 중복 경로는 제거. brew formula/cask는 네이티브 uninstall 명령 + 역의존성(`brew uses --installed`) 경고 + autoremove 제안, GUI 앱은 Bundle ID 정확 일치 잔여물(높은 확신) + 이름 기반(낮은 확신) 구분. 이름 매칭은 **정확 일치 우선**(`matchApps` 순수 함수) — 정확 일치가 없을 때만 부분일치를 쓰고 타깃에 "이름 부분일치" 경고를 붙인다. 순수 함수 `IsProtected`(시스템 경로 블랙리스트)와 `IsAppleSystemApp`(`com.apple.*` — Safari처럼 /Applications에 있는 애플 앱 보호)로 안전 강제, `MoveToTrash`는 실행 직전 `IsProtected` 재확인. **아래 안전 규칙 필수 준수.**

6. **안전성·구조 정비** ✅ — disk 기능을 얹기 전에 바닥을 고쳤다. 상세는 각 항목이 서술된 곳(아키텍처 / clean 안전 규칙 / TUI)에 통합했고, 여기서는 **무엇을 왜 고쳤는지**만 남긴다.
   - **6-1 Bundle ID 접두사 과매칭** [데이터 손실 가능] — `com.google.Chrome` 삭제 계획이 `com.google.Chrome.canary` 경로를 [확실]로 삼켰다. → `excludeOtherAppIDs`.
   - **6-2 glob 메타문자** — 이름에 `[`/`*`/`?`가 든 앱이 무관한 경로를 매칭했다. → 이름 경로는 리터럴 확인.
   - **6-3 레이어 역전** — `source/apps.go`가 `clean`을 import했다. → `internal/paths` 추출. 리팩터 전후 `scan --json`(용량/시각 제외)·`clean` dry-run 13건·`orphans` 출력이 **완전히 동일**함을 diff로 검증했다.
   - **6-4 du 이중 계산** — 부모/자식이 함께 `du`에 넘어가면 자식이 두 번 세졌다(부모가 나중에 추가되는 방향만 방어가 없었다). → `paths.DedupContained`(양방향 + 부모 추가 시 기존 자식 제거).
   - **6-5 표시 용량 ≠ 회수 가능량** — 아래 별도 항목.
   - **6-6 scan 진행률** — `dataKB`가 앱당 300초 타임아웃이라 멈춘 것처럼 보였다. → stderr 진행률.
   - **6-7 TUI `enter` 분리** — 목록 enter + 모달 enter = 두 번에 실제 삭제. → `d`/`u`만.
   - **6-8 `Target.Argv`** — 명령 문자열을 실행 직전 `strings.Fields`로 재분해해 공백 인자가 깨졌다.
   - **6-9 osascript argv 전달** — `%q` 이스케이프가 AppleScript와 어긋났다.
   - **6-10 권한 사전 확인** — `/Library` 대상은 실패가 예정돼 있었다. → `NeedsAdmin` 분리.

7. **disk** ✅ — 이 도구의 원래 동기(시스템 설정의 "문서" 57GB인데 Finder로는 11GB만 보였고, 원인이 `~/.ollama` 모델과 개발 캐시였다). `macinv disk`.
   - **`data/locations.json`**(`go:embed`): `id`/`name`/`globs`(`~` 확장)/`tool`(소유 도구)/`tier`/`cleanup`(**Argv**)/`trash`/`note`. 네이티브 정리 명령이 없는 위치는 `cleanup`을 비우고 `note`로 이유를 남긴다(테스트가 강제한다 — 사용자를 막다른 길에 두지 않기 위해).
   - **등급**은 "지워도 되는가"가 아니라 **"지우면 무엇을 잃는가"**다: `safe`(재생성됨) / `artifact`(빌드 산출물) / `expensive`(재다운로드 비쌈) / `never`(소스·사용자 데이터). `Cleanable(tier, includeExpensive)`가 유일한 판정 — 기본은 safe+artifact, expensive는 `--include-expensive`, **`never`는 플래그로도 뚫리지 않는다**(테스트로 고정).
   - **네이티브 명령이 없으면 휴지통 이동**(`trash: true`, DerivedData 등). rm은 어떤 경우에도 쓰지 않는다. 대신 "휴지통을 비우기 전까지 실제로는 안 빈다"를 계획에 명시한다.
   - **고아 캐시 교차 검증**(`orphan.go`): 캐시는 있는데 그걸 쓰는 것이 하나도 안 남았으면 `[고아 캐시]`. `du`/`ncdu`는 "폴더가 크다"까지만 말하고 **"이건 이제 아무도 안 쓴다"는 인벤토리를 가진 macinv만 할 수 있는 판단**이라 요약에서 별도 섹션으로 낸다. 판정은 순수 함수 `IsOrphanCache`(Tool을 모르거나 never면 판정하지 않는다).
     - **오탐 방지 3중 장치 — 하나라도 빼면 멀쩡한 캐시가 고아로 뜬다:**
       ① **인벤토리**(`InstalledTools`) — brew/npm/pipx/cargo/gobin/앱/런타임 이름 + 앱 Bundle ID 마지막 마디.
       ② **PATH 2차 확인**(`DetectTools`의 `exec.LookPath`) — 인벤토리 캐시가 오래돼 runtimes가 비면 python이 없다고 보고 pip 캐시를 오탐한다(실측). `toolBinaries`로 이름이 다른 실행 파일도 본다(pip→pip3, simctl→xcrun).
       ③ **`owner_apps`**(locations.json) — **CLI 도구 없이 GUI 앱만 쓰는 경우**. gradle CLI를 안 깔아도 Android Studio는 `~/.gradle`을 쓰고, mvn 없이도 IntelliJ는 `~/.m2`를 쓴다. 이 케이스는 ①②로 절대 못 잡는다.
     - 이름 정규화는 `toolKeys`가 **넣을 때와 찾을 때 모두** 담당한다("Android Studio" → `android studio`/`androidstudio`). 한쪽만 바꾸면 매칭이 조용히 깨져 오탐이 돌아온다.
     - `toolAliases`(pip←python 등)도 오탐 방지용이니 **완화 금지**. 이 규칙들은 전부 실기 오탐에서 나왔거나(①②) 나올 수밖에 없는 구조(③)라 테이블 테스트로 고정돼 있다.
   - **Ollama는 위치 단위로 지우지 않는다** — 모델 하나가 수 GB이고 재다운로드가 비싸서 `ollama list`로 모델별 용량을 보여주고 `ollama rm <model>`을 개별 선택으로 만든다(`BuildOllamaPlan`).
   - **`--deep`**(`deep.go`): 기본 스캔은 알려진 위치만 훑어 3초 내(실측 1.3초). `--deep`일 때만 홈에서 오래된 `node_modules`를 찾는다 — 깊이 6, 숨김 디렉토리·`Library`·미디어·클라우드 동기화 폴더 제외, **최종 수정 6개월 이상만**, 전체 예산 120초.
   - **APFS 로컬 스냅샷**(`snapshot.go`): `tmutil listlocalsnapshots /` + `df -k /System/Volumes/Data`로 "지워도 여유 공간이 안 느는 이유"를 **안내만** 한다. `tmutil deletelocalsnapshots`는 백업 복원 지점을 없애는 행위라 이 도구가 대신 결정하지 않는다.
   - **회수 예상**: 디스크 계획은 회수량 대부분이 휴지통 경로가 아니라 **명령**(brew cleanup 등)에서 나온다. `clean.Plan.ExtraReclaimKB`에 그 몫을 실어 `ReclaimLine`이 요약의 "정리 시 회수 가능"과 같은 숫자를 내게 한다(안 그러면 3.1 GB vs 139 MB로 어긋난다 — 실측).
   - **Electron 앱 캐시**(2026-08-03 추가): Notion 캐시가 66GB까지 자란 사례가 동기. Electron 앱은 이름만 다를 뿐 같은 Chromium 캐시 구조를 쓰므로 **앱을 하드코딩하지 않고 `~/Library/Application Support/*/...` 패턴 하나로** 잡는다(`Cache`/`Code Cache`/`GPUCache`/`Service Worker/CacheStorage`). tier `safe` + `trash`(네이티브 정리 명령이 없다).
     - **`IndexedDB`·`Local Storage`·`Cookies`·`Session Storage`는 절대 포함하지 않는다** — 로그인 세션과 로컬 데이터가 들어 있어 지우면 재로그인이 필요하다. 이건 캐시 정리가 아니라 데이터 삭제다. glob 금지 목록을 **테스트로 강제**했으니(`TestElectronLocationNeverTargetsUserData`) 편의를 위해 추가하지 말 것.
     - **`Partitions/<파티션>/` 아래도 본다**(2026-08-04): 세션을 분리하는 앱(워크스페이스를 여러 개 쓰는 Notion·Slack, Claude의 preview 파티션)은 `<앱>/Partitions/<파티션>/` 아래에 **같은 캐시 구조를 또** 만든다. 한 단계만 보면 통째로 놓친다. `Service Worker/ScriptCache`도 같은 이유로 추가. **제외 원칙(IndexedDB·Local Storage·Cookies)은 Partitions 아래에서도 동일**하다. `appSupportDirName`은 base 다음 **첫 마디**를 쓰므로 Partitions 경로에서도 앱 이름을 뽑는다 — Breakdown이 파티션별로 쪼개지지 않고 앱 단위로 합쳐진다(`TestBreakdownAggregatesPartitionsPerApp`로 고정).
     - 앱 단위 `clean` 계획과 경로가 겹쳐도 `ResolvePaths`의 `paths.DedupContained`가 더 넓은 위치 하나만 남긴다(`TestElectronPathsNeverNest`로 고정).

   - **실행 중인 앱은 캐시를 지우지 않는다**(`running.go`, 2026-08-04): Chromium은 캐시 파일 핸들을 **연 채로** 돈다. 켜져 있을 때 캐시 디렉토리를 휴지통으로 옮기면 앱이 오작동하거나 캐시를 이상하게 재생성한다. Notion·Slack을 켜둔 채 `disk --apply`를 돌리는 게 가장 흔한 상황이라 실사용에서 바로 부딪힌다.
     - 탐지는 `ps -Axo comm=` **1회**(경로마다 부르지 않는다) → `ParseRunningApps`가 경로에서 `.app` 번들 이름을 뽑는다. 프로세스 이름(`Electron`, `Code Helper`)이 아니라 **앱 번들 이름**을 쓰는 게 훨씬 믿을 만하다. 한 경로에 `.app`이 중첩되면(헬퍼) **전부** 담는다 — 놓쳐서 지우는 것보다 넉넉히 잡아 제외하는 쪽이 안전하다.
     - 매칭은 순수 함수 `MatchesRunningApp`: ① 정규화 완전 일치 → ② 별칭 표(`dirToAppAliases`) → ③ 앱 이름을 단어로 쪼갰을 때 디렉토리 이름이 **한 단어로** 들어 있음("Code" ⊂ "Visual Studio Code", "LogiOptionsPlus" ⊂ "logioptionsplus_agent"). **부분 문자열이 아니라 단어 단위**인 이유는 "Home"/"Mail" 같은 짧은 이름이 무관한 앱에 걸리면 멀쩡한 캐시가 영영 정리되지 않기 때문. 다만 애매하면 **제외(보호) 쪽으로 기운다**.
     - 기본은 제외 + `Target.Excluded`에 "실행 중(<앱>) — 앱을 종료한 뒤 다시 실행하세요". `--include-running`이면 포함하되 **앱마다 한 번씩** 경고를 붙인다(경로마다 반복하면 소음). TUI는 해제 수단이 없다(기본 보호 고정).
     - 요약 하단에 `⏸ N개 앱이 실행 중이라 제외됨 (총 X GB)` + 앱별 내역을 낸다 — 앱을 끄고 다시 돌릴지 판단하려면 얼마가 걸려 있는지 알아야 한다.
     - **조회 실패는 "실행 중인 앱 없음"과 반드시 구분한다**(`RunningApps`가 `(apps, ok)`를 돌려주고 `Result.RunningCheckFailed`에 남는다). 예전엔 실패 시 nil을 돌려줘 보호가 **조용히 꺼졌다** — 사용자는 보호가 도는 줄 알고 `--apply`를 누르는데 실행 중인 앱의 캐시가 그대로 지워진다. **안전장치가 조용히 사라지는 건 처음부터 없는 것보다 나쁘다.** 실패했고 Electron 캐시가 **계획에 실제로 담겼을 때만** `clean.Plan.Warnings`로 계획 상단에 경고를 띄우고(무관한 계획에는 소음이 안 되게), 리포트·`--json`(`running_check_failed`)에도 남긴다. ps 출력이 통째로 비는 것도 실패로 본다(macOS에서 프로세스가 0개일 수는 없다).

   - **비용**: 이 맥 실측 기본 1.3초. 가장 무거운 건 OrbStack 데이터(26GB) walk다.

## clean 안전 규칙 (모든 삭제/정리 커맨드에 적용)

- **기본은 dry-run**: 삭제 계획(파일 목록 + 사용할 명령)만 출력. 실제 삭제는 `--apply` + 대화형 확인 필요.
- **rm 금지, 휴지통 이동**: `/usr/bin/trash`(있으면) 또는 `osascript`로 Finder에게 삭제 위임. 사용 가능 여부를 먼저 확인.
  - osascript 폴백은 경로를 **argv로 넘긴다**(`-e 'on run argv'` … `-e 'end run'` … `-- <path>`). 스크립트 본문에 `fmt.Sprintf`+`%q`로 끼워 넣으면 Go 이스케이프 규칙(한글이 `\uXXXX`로 나감)이 AppleScript와 어긋난다. **주의**: `POSIX file <변수>`는 리터럴과 달리 `as alias` 강제 변환이 없으면 Finder가 -1728로 실패한다(실측).
- **권한 없는 경로는 계획에서 분리**: 부모 디렉토리에 쓰기 권한이 없으면 삭제하지 말고 "권한 필요 — 수동 처리"로 표시. 실패할 걸 알면서 시도해 ✗ 로그만 남기지 않는다.
- **시스템 경로 하드코딩 블랙리스트**: `/System`, `/usr`, `/bin`, `/sbin`, `/Library/Apple`은 절대 접근 금지.
- **잔여 파일 매칭은 Bundle ID 정확 일치 우선**: `mdfind kMDItemCFBundleIdentifier` 기반. 이름 유사도 기반 매칭 결과는 "확신 낮음"으로 별도 표시.
- **접두사 glob이 다른 앱을 삼키지 않게 한다**: `<bundleID>*` 매칭 결과 중 파생 ID가 다른 설치 앱의 Bundle ID에 귀속되면 제외(위 `excludeOtherAppIDs`). [확실]은 `--apply` 기본 삭제 대상이라 여기 말고는 방어선이 없다.
- **앱 이름은 glob 패턴이 아니다**: 이름 기반 경로는 `filepath.Glob`이 아니라 **리터럴 존재 확인**으로 찾는다. 이름에 `[`/`*`/`?`가 있으면 문자 클래스·와일드카드로 해석돼 무관한 경로를 삼킨다(`"Foo [beta]"` → `~/Library/Caches/Foo b`). 패턴에 끼워 넣는 가변 부분(Bundle ID)은 `escapeGlobMeta`로 이스케이프한다.
- **brew 삭제**: `brew uses --installed`로 역의존성 확인 후 경고, 삭제 후 `brew autoremove` 제안.

## 마일스톤 이후 추가 기능 및 개선

**업데이트 서브시스템**(`internal/clean/upgrade.go` + `internal/cmd/update.go`): `BuildUpgradePlan`/`BuildCombinedUpgradePlan`이 `brew upgrade`·`brew upgrade --cask`·`npm install -g <pkg>@latest`·`mas upgrade <id>` 명령을 만든다(삭제와 `Plan`/`Execute`/`RenderPlanText` 공유, `Plan.Verb`/`Icon`으로 제목만 "삭제"↔"업데이트" 전환). CLI `macinv update <name>`(dry-run 기본, `--apply`)와 TUI `u` 키가 한 코드 경로. **npm 셀프 업데이트는 반드시 `npx -y npm@latest install -g npm@latest`로**(`npmUpgradeTarget`) — 실행 중인 npm이 자기 모듈을 지우다 MODULE_NOT_FOUND로 죽는 실측 버그 회피.

**orphans 서브시스템**(`internal/clean/orphans.go` + `internal/cmd/orphans.go`): 설치 앱 Bundle ID와 `~/Library` 항목을 대조해 **지워진 앱의 고아 데이터**를 찾는다(`macinv orphans`, dry-run 기본). 판정은 순수 함수 `classifyOrphan` — Bundle ID 형태(점 2개 이상)만 대상, `orphanProtected`(`com.apple.` **포함**·`systemgroup.`·팀ID 접두사 `^[A-Z0-9]{10}\.`·`is.workflow.`/`org.sparkle-project.`) 보존, 설치 ID 양방향 접두사 보존, 벤더 공유 보존. 이 보호 규칙들은 실기 오탐(Apple Podcasts 그룹·단축어·Sparkle)에서 나온 것이니 **완화 금지**. `/Library`는 스캔하지 않는다(타 사용자/서비스 소유 가능). 확신도는 전부 `[확신중간]`.

**update --all**: `collectOutdatedRows`(cmd/outdated.go, outdated 커맨드와 공용) → `clean.UpgradePlanFromOutdated`(순수 함수 — 소스를 이미 알아 프로브 불필요, mas는 `OutdatedRow.ID` 사용, npm 셀프는 npx 규칙 자동). 확인 입력은 `all`.

**점검 후속 개선(2026-07-20)**: ① `clean` 앱 이름 매칭은 정확 일치 우선(위 마일스톤 5에 통합 서술). ② TUI는 시작 스캔과 재스캔 병합 직후 `PersistFunc`로 `~/.macinv/inventory.json` 캐시를 동기화한다(persist는 Update 루프에서 동기 호출 — m.rep 경합 방지). ③ 잡 완료 시 실패 건수 요약(`jobDoneMsg.failed`)을 로그에 남긴다. ④ 업그레이드/설치 명령만 `Target.CmdTimeoutSec=1800`(30분), 삭제·휴지통은 기본 300초.

**앱 용량 = 번들 + 데이터(2026-08-02)**: 예전엔 `du -sk`를 `.app` 번들에만 걸어 "용량 큰 앱 Top 10"이 **설치 파일 크기 랭킹**이었다. Electron 앱은 번들 283MB인데 캐시가 67GB인 식이라 실제 디스크 점유가 전혀 안 보였다. 이제 `appsSource.dataKB`(`apps.go`)가 `clean.SizePaths`로 앱 데이터 경로를 모아 **`du -sk`에 한 번에 넘겨**(앱당 프로세스 1회) 잰다 → `report.App.DataSizeKB`(`data_size_kb,omitempty`), 정렬/표시는 `App.TotalSizeKB()`(번들+데이터). 요약은 `report.TopBySize`(순수 함수)로 뽑고 내역("앱 283 MB + 데이터 67.2 GB")을 함께 보여준다. TUI 앱 탭 용량 칸도 합계.
- **`clean.SizePaths`(`clean/size.go`)는 측정 전용이며 삭제에 쓰지 않는다.** `LeftoverPaths`(=삭제 계획과 동일한 규칙)에 두 가지를 더한다: ① 벤더 폴더 내 앱 하위 폴더(Chrome 데이터는 `Application Support/Google/Chrome` — Bundle ID로도 앱 이름으로도 안 잡힘, 공유 컴포넌트는 제외) ② 그룹 컨테이너 이름 매칭(`groupContainerMatchesApp` — 팀ID 접두사를 떼고 **마지막 마디만 정확 비교**; OrbStack은 Bundle ID가 `dev.kdrag0n.MacVirt`라 `HUAQ24HBR6.dev.orbstack` 24GB를 ID로는 영영 못 찾는다). 부분일치를 안 쓰는 이유는 "Mail"/"Home" 같은 짧은 이름이 무관한 컨테이너를 쓸어담기 때문 — **완화 금지**. 이미 잡힌 경로의 하위 경로는 중복 계산에서 제외한다.
- **비용**: 데이터 `du`가 스캔에서 가장 무겁다(이 맥 실측 0.9s → 9.4s, 대부분 수십 GB 캐시 트리 walk). 급하면 `--no-sizes`. 진행률은 stderr에 표시된다(6-6).

**표시 용량 ≠ 회수 가능량(2026-08-02, 6-5)**: 앱 목록의 용량은 `paths.SizePaths`(측정용 **넓은** 규칙 — 벤더 폴더 포함)로 재는데, 삭제 계획은 벤더 공유 여부를 따로 판정해 그 경로를 보존할 수 있다. 즉 "Chrome 10.9 GB"를 보고 지웠는데 2 GB만 비는 상황이 **정상**이다. 예전엔 이 간극이 코드 주석에만 있었다. 이제 UI에 드러낸다:
- 용량 표시는 합계 옆에 **내역**을 붙인다(`번들 2.0 GB + 데이터 8.8 GB`). 순수 함수 `report.SizeBreakdown` — CLI 요약(Top 10)과 TUI 앱 탭이 공유. 번들은 앱을 지워야 줄고, 데이터는 clean으로 줄일 여지가 있다는 신호다.
- 계획 하단에는 **계획에 실제로 담긴 TrashPaths만** 합산한 `💾 이번 삭제로 회수 예상: N GB`를 따로 낸다(`clean.ReclaimLine`). [확신낮음] 제외가 기본(= `--apply` 기본값)이고, 모두 포함했을 때의 상한을 괄호로 같이 보여준다. TUI 모달은 체크박스 토글 상태로 **즉시 다시 계산**한다. `NeedsAdmin`(권한 필요) 경로는 회수량에 넣지 않는다 — 어차피 안 지워진다.

**성능 최적화(2026-07-20)**: ① 앱 스캔의 mdls를 앱당 3회→1회(`parseMdlsKeyed`, 위 "알려진 주의점"에도 서술). ② `BuildUpgradePlan`(4개 프로브)·`BuildPlan`(brew 2개)·`appendPackageTargets`(3개) 프로브를 병렬화 — TUI 모달 대기 절반 이하. ③ 벤더 판단용 `installedAppBundleIDs`는 `sync.OnceValue`로 지연(벤더 매칭 시에만) + 내부 mdls 병렬(worker 8). 측정: scan 2.7s→2.0s, update 계획 1.45s→0.65s, 출력 동일 검증.

## 테스트 관례
- 파싱 로직마다 테이블 기반 유닛 테스트. 실제 명령 출력 샘플을 `internal/source/testdata/`에 fixture로 저장하고, 파서 함수에 직접 주입해 검증(exec 미실행).
- 새 소스/커맨드 추가 시 프로토타입 JSON과 구조적으로 diff해 누락 필드가 없는지 확인하는 습관을 유지한다(M1에서 검증한 방식).
- 안전 판정은 전부 순수 함수 — 파일시스템/외부 명령 없이 테이블 테스트로 커버한다. `paths`: `IsProtected`/`IsAppleSystemApp`/`bundleGlobs`/`nameGlobs`/`DerivedBundleID`/`DedupContained`/`groupContainerMatchesApp`/`LookupVendor`. `clean`: `classifyOrphan`/`matchApps`/`excludeOtherAppIDs`/`ownerOfID`/`splitByWritability`/`ShellQuote`/`ReclaimKB`/`parseDuSizes`. `source`(EOL): `MatchCycle`/`normalizeVersion`/`eolDateStatus`/`compareVersions`/`SuggestUpgradeCommand`/`applyEOL`, `report`: `EOLBadge`/`UpdateBadge`. `clean`: `RuntimeMajorJump`/`cyclePrefix`/`SplitRuntimeMajors`. `disk`: `Cleanable`/`IsOrphanCache`/`InstalledTools`/`toolKeys`/`ExpandGlob`/`ResolvePaths`/`HasAppleComponent`/`GroupUsage`/`BreakdownLine`/`ParseRunningApps`/`MatchesRunningApp`/`RunningBlockedUsage`/`BuildCleanupPlan`/`BuildOllamaPlan`/`ParseOllamaList`/`ParseSnapshots`/`ParseDF`/`SumDuKB`. 파일시스템이 필요한 것(`gatherLeftovers`, `SizePaths`, `MeasureTrashSizes`)은 `t.TempDir()`에 실제 파일을 만들어 검증한다.
- 실기에서 오탐/오탐지가 나오면 반드시 회귀 테스트로 고정한다(orphans의 `orphanProtected`, 6-1의 `TestLeftoversDoNotSwallowOtherApp`, 6-4의 `TestDedupContained`가 그 예).
- **리팩터링은 출력 diff로 검증한다**: 동작이 바뀌면 안 되는 변경(6-3 같은 레이어 이동)은 리팩터 전 바이너리를 `git worktree`로 빌드해 두고 `scan --json`/`clean` dry-run/`orphans` 출력을 diff한다. 용량(`size_kb`/`data_size_kb`)과 `generated_at`은 실행마다 달라지므로 비교에서 뺀다.

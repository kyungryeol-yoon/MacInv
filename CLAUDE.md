# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트

`macinv` — macOS에 설치된 소프트웨어 전체를 스캔해 "내 맥에 뭐가 어디 깔려있는지" 파악하고, 업데이트 대기를 표시하고, 최종적으로 잔여물 없이 안전하게 삭제하는 Go CLI. 검증된 Python 프로토타입 `mac_inventory.py`(참조용으로 보존)를 Go로 재설계한 것이다. 사용자 대상 출력은 **한국어**(이모지 배너 포함).

- Go 모듈: `github.com/kyungryeol-yoon/macinv` (현재 로컬 개발만, GitHub 저장소는 미생성)
- Go 1.22+ (개발 환경은 brew의 go 1.26). 외부 의존성은 `spf13/cobra`, `golang.org/x/sync/errgroup`만.

## 명령어

```bash
go build ./...            # 빌드
go vet ./...              # 정적 검사
go test ./...             # 전체 테스트
go test ./internal/source -run TestParseMas   # 단일 테스트
go run . scan             # 전체 스캔 + 요약 (실제 실행)
go install .              # ~/go/bin/macinv 로 설치 (macinv --version)
```

`macinv scan` 플래그: `--json`(JSON을 stdout으로), `--no-outdated`(네트워크 최신버전 체크 생략), `--no-sizes`(du 용량 계산 생략), `--no-runtimes`(언어 런타임 조회 생략), `--quiet`(요약 생략), `-o PATH`(저장 경로). 결과는 항상 `~/.macinv/inventory.json`에 캐시된다. scan은 이제 **설치된 언어 런타임 버전도 요약에 포함**한다.

## 아키텍처

3-레이어로 스캔/데이터/출력을 분리한다 (TUI를 나중에 bubbletea로 얹기 위한 구조):

- **`internal/source/`** — 스캔 로직. 각 소스(apps/brew/npm/pipx/cargo/mas/gobin/launchd)가 `Source` 인터페이스(`Name() / Available() / Scan(ctx)`)를 구현한다. `source.Default()`가 기본 소스 목록을, `source.ScanAll()`이 `errgroup`으로 **병렬 스캔**해 이름→결과 맵을 돌려준다. 새 소스(gem, mise 등) 추가 = 파일 하나 + `Default()`에 한 줄.
  - **파싱은 exec와 분리**되어 있다. `Scan`은 명령을 실행하고, 순수 파서 함수(`parseBrewInfo`, `parseNpmLs`, `parseMas`, `parseCargoList`, `parseLaunchdJSON`, `applyBrewOutdated`, `applyNpmOutdated` 등)가 바이트/문자열을 받아 report 타입을 돌려준다. 테스트는 이 파서들을 fixture(`internal/source/testdata/`)로 직접 검증한다.
  - `Available()`이 false(명령 미설치 / 디렉토리 없음)면 스캔을 건너뛰고 결과는 nil → JSON `null`. **미설치는 에러가 아니라 조용한 스킵**이다.
- **`internal/exec/`** — 외부 명령 공통 실행기 `Runner`. 모든 호출에 `context.WithTimeout` 적용. 실패해도 panic하지 않고 `Recorder`에 경고를 모아 리포트 `errors` 필드로 내보낸다. 종료코드가 허용 밖이어도 **stdout이 있으면 성공**으로 본다(프로토타입 규칙). `npm ls`/`npm outdated`처럼 정상 상황에 exit 1을 내는 명령은 `Opt.OKCodes: []int{0,1}`로 허용한다. 실행 파일 자체가 없으면 `ErrNotFound`(경고 없음).
- **`internal/report/`** — JSON 스키마 타입(프로토타입 준수)과 한국어 요약 렌더(`PrintSummary`). null 가능 필드는 포인터(`*string`/`*int`/`*bool`)로 두어 JSON에서 `null`로 나간다. `Ptr`/`PtrOrNil` 헬퍼 사용. **런타임/소유권 데이터 타입(`Runtime`/`Origin`/`VersionManager`)도 여기 있다** — source가 report를 import하므로 순환 없이 source(로직)/tui/cmd가 공유한다(source는 `type Runtime = report.Runtime` 별칭으로 노출).
- **`internal/clean/`** — 삭제 플래너. `BuildPlan`은 읽기 전용으로 계획만, `Execute`(휴지통 이동)는 cmd가 `--apply`+확인 후에만 호출. 안전 판정은 순수 함수 `IsProtected`/`IsAppleSystemApp`.
- **`internal/tui/`** — bubbletea 대화형 뷰(`tui.go` 상태/흐름, `view.go` 렌더/탭). 탭: 앱/brew/**패키지(npm·pipx·cargo·go)**/런타임/업데이트/launchd.
  - **실행 가능(Phase A)**: `space` 멀티선택 → `d` → **계획 모달**(CLI와 **동일한 `clean.PlanLines`/`RenderPlanText` 공유**) → `[확신낮음]`은 체크박스 기본 해제(`space` 토글) → GUI 앱은 `y/N` 추가 확인, brew/npm 등은 승인 즉시 → `clean.Execute`를 goroutine에서 실행, 각 줄을 `chan tea.Msg`→`waitEvent` 루프로 **하단 로그 패널에 실시간 스트리밍**(UI 비블로킹), **동시 1개만 큐잉**. 완료 후 `RescanFunc`(cmd가 `RescanSources`로 주입)로 **영향 소스만 부분 재스캔**해 목록 갱신. `r`=전체 재스캔, `esc`=대기 큐 취소(실행 중 외부 명령은 강제 종료 못 함). 안전 규칙은 전부 `clean` 재사용. **선택형**: ↑/↓로 행 선택(수동 스크롤 관리), ←/→ 탭 전환, **d로 선택 항목 삭제**(`clean.BuildPlan`→계획 확인 뷰→`y`→`clean.Execute`, 기본 `[확신낮음]` 제외). 모드(browse/busy/plan/result)와 계획 생성/실행은 `tea.Cmd`로 비동기. TTY 없이 모델 전이·네비게이션·삭제 프롬프트를 테스트한다.
  - **탭 라벨 카운트**(`tab.count`)는 `len(tab.rows)`와 **다르다** — rows에는 "(없음)"/"✅ 최신 상태" 같은 안내 플레이스홀더 행이 섞여 있어, 헤더 숫자는 `buildTabs`에서 실제 데이터 길이(`brewCount`/`packageCount`/`installedRuntimeCount`/`len(outdated)` 등)로 **별도 계산**한다. 스크롤/선택 clamp는 여전히 `len(rows)` 사용(플레이스홀더도 화면엔 한 줄 차지하므로). 새 탭 추가 시 이 둘을 혼동하지 말 것 — 실제로 업데이트가 0개인데 "업데이트 (1)"로 뜨던 버그가 이 혼동에서 나왔다.
- **`internal/cmd/`** — cobra 커맨드(출력 레이어). 스캔 수집은 `collectInventory`(scan/tui 공용)로 모았다: `buildSections` 조립 + `--no-outdated`가 아니면 `EnrichBrew`/`EnrichNpm` 보강 + `--no-runtimes`가 아니면 `ScanRuntimes`/`ScanVersionManagers`. 결과는 `report.Report`(runtimes/version_managers는 `omitempty`).

### 알려진 주의점
- **mdls swap 버그**: 프로토타입은 `mdls -raw`에 여러 `-name`을 한 번에 넘겨 반환 순서가 뒤바뀌어 앱의 `version`/`date_added`가 서로 바뀌어 저장됐다. Go판은 속성을 **하나씩** 조회해(`mdlsOne`) 이 버그를 고쳤다. 프로토타입 JSON과 diff 시 이 두 필드 값이 다른 것은 정상.
- JSON 스키마는 프로토타입과 **키 단위로 완전 일치**했으나(M1 검증), 이후 사용자 요청으로 `runtimes`/`version_managers` 최상위 필드를 **의도적으로 추가**했다(`omitempty`라 outdated/trace 등 다른 커맨드 출력엔 안 나옴).

## 마일스톤

진행은 마일스톤 단위. 각 단계마다 이 맥에서 실제 실행 → 결과 확인 → 다음.

1. **scan** ✅ (완료) — 전체 스캔 + 요약 + JSON 캐시.
2. **outdated** ✅ (완료) — brew/npm/mas 업데이트 대기 전용 뷰 (`--json`). `source.OutdatedSources`로 3개 소스만 스캔 후 보강, `report.OutdatedRows`로 행 집계(요약과 공용), `text/tabwriter`로 표 출력.
3. **trace `<name>`** ✅ (완료) — `which -a` 각 경로를 `filepath.EvalSymlinks`로 해소해 소유권 판별. 순수 함수 `source.Classify`(경로만 보고 brew Cellar/Caskroom · npm node_modules · pipx venvs · cargo · go bin · system · manual 판별)로 분리 → **M4 runtimes에서 재사용**. PATH 첫 경로를 Active(★)로 표시. `--json` 지원. (현재 nvm/pyenv 관리 바이너리는 "manual"로 나옴 — 버전 매니저 감지는 M4에서 보강.)
4. **runtimes** ✅ (완료) — node/python/go/rust/java/ruby/php/dotnet/swift의 활성 버전+경로+관리주체를 병렬 조회. 버전은 `exec.RunCombined`(stdout+stderr 합침, java/swift 대응)로 얻어 `extractVersion`(순수 함수)으로 추출, swift는 `swift-driver` 버전 오인을 막는 전용 정규식 사용. 관리 주체는 `ClassifyManaged`(nvm/pyenv/mise/asdf/rustup/sdkman 경로 우선 → `Classify` 폴백, `TraceManaged` 경유). 서로 다른 실제 경로가 2개 이상이면 `Conflict`로 ⚠️ 표시. 버전 매니저는 `ScanVersionManagers`가 홈 디렉토리 glob(`~/.nvm/versions/node/*` 등)만으로 관리 버전을 열거(외부 명령 없음). `--json` 지원.
**추가 작업(마일스톤 이후)** ✅ — `scan`에 런타임 통합, `tui` 커맨드(bubbletea), `README.md`, `go install`(버전 0.1.0, `macinv --version`), clean에 npm/pipx/cargo/go·벤더폴더 처리, **TUI 실행화(멀티선택·삭제·업데이트·스트리밍·부분재스캔)**, **`macinv update`(업그레이드 플래너)**.

**업데이트 서브시스템**(`internal/clean/upgrade.go` + `internal/cmd/update.go`): `BuildUpgradePlan`/`BuildCombinedUpgradePlan`이 `brew upgrade`·`brew upgrade --cask`·`npm install -g <pkg>@latest`·`mas upgrade <id>` 명령을 만든다(삭제와 `Plan`/`Execute`/`RenderPlanText` 공유, `Plan.Verb`/`Icon`으로 제목만 "삭제"↔"업데이트" 전환). CLI `macinv update <name>`(dry-run 기본, `--apply`)와 TUI `u` 키가 한 코드 경로. **npm 셀프 업데이트는 반드시 `npx -y npm@latest install -g npm@latest`로**(`npmUpgradeTarget`) — 실행 중인 npm이 자기 모듈을 지우다 MODULE_NOT_FOUND로 죽는 실측 버그 회피.

**점검 후속 개선(2026-07-20)**: ① `clean` 앱 이름 매칭은 **정확 일치 우선**(`matchApps` 순수 함수) — 정확 일치가 없을 때만 부분일치를 쓰고 타깃에 "이름 부분일치" 경고를 붙인다. ② TUI는 시작 스캔과 재스캔 병합 직후 `PersistFunc`(cmd의 `PersistReport`)로 `~/.macinv/inventory.json` 캐시를 동기화한다(persist는 Update 루프에서 동기 호출 — m.rep 경합 방지). ③ 잡 완료 시 실패 건수 요약(`jobDoneMsg.failed`)을 로그에 남긴다. ④ 업그레이드/설치 명령만 `Target.CmdTimeoutSec=1800`(30분), 삭제·휴지통은 기본 300초.

**성능 최적화(2026-07-20)**: ① 앱 스캔의 mdls를 앱당 3회→1회(`parseMdlsKeyed` — "키 = 값" 키 기반 파싱이라 -raw의 순서 swap 버그와 무관). ② `BuildUpgradePlan`(4개 프로브)·`BuildPlan`(brew 2개)·`appendPackageTargets`(3개) 프로브를 병렬화 — TUI 모달 대기 절반 이하. ③ 벤더 판단용 `installedAppBundleIDs`는 `sync.OnceValue`로 지연(벤더 매칭 시에만) + 내부 mdls 병렬(worker 8). 측정: scan 2.7s→2.0s, update 계획 1.45s→0.65s, 출력 동일 검증.

**orphans 서브시스템**(`internal/clean/orphans.go` + `internal/cmd/orphans.go`): 설치 앱 Bundle ID와 `~/Library` 항목을 대조해 **지워진 앱의 고아 데이터**를 찾는다(`macinv orphans`, dry-run 기본). 판정은 순수 함수 `classifyOrphan` — Bundle ID 형태(점 2개 이상)만 대상, `orphanProtected`(`com.apple.` **포함**·`systemgroup.`·팀ID 접두사 `^[A-Z0-9]{10}\.`·`is.workflow.`/`org.sparkle-project.`) 보존, 설치 ID 양방향 접두사 보존, 벤더 공유 보존. 이 보호 규칙들은 실기 오탐(Apple Podcasts 그룹·단축어·Sparkle)에서 나온 것이니 **완화 금지**. `/Library`는 스캔하지 않는다(타 사용자/서비스 소유 가능). 확신도는 전부 `[확신중간]`.

**update --all**: `collectOutdatedRows`(cmd/outdated.go, outdated 커맨드와 공용) → `clean.UpgradePlanFromOutdated`(순수 함수 — 소스를 이미 알아 프로브 불필요, mas는 `OutdatedRow.ID` 사용, npm 셀프는 npx 규칙 자동). 확인 입력은 `all`.

**exec 실행 규칙 주의**: 스캔(읽기)은 "stdout 있으면 비허용 종료코드라도 성공"(프로토타입 계승)이지만, **변경 명령(clean/update의 `Execute`·`MoveToTrash`)은 `Opt.Strict`로 엄격 판정** — 부분 실패를 성공으로 오인하지 않는다. 실패 에러 메시지에는 stderr(없으면 stdout) 상세가 포함된다.

5. **clean `<name>`** ✅ (완료, 이후 강화) — 삭제 플래너. `internal/clean` 패키지. `BuildPlan`은 **읽기 전용**으로 계획만 만들고, 실제 삭제는 `Execute`(휴지통 이동)를 `--apply` + 대화형 확인(대상 이름 정확 입력) 후에만 호출.
   - **잔여물 탐지는 glob 접두사 매칭**(`bundleGlobs`/`nameGlobs`/`gatherLeftovers`)이라 헬퍼·파생 파일(`<bundleID>.ShipIt`, `<bundleID>.helper.plist` 등)과 Group Containers·Cookies·ByHost·시스템 `/Library`까지 잡는다.
   - **확신도 3단계**: `[확실]`(Bundle ID, `ConfHigh`) / `[확신중간]`(`ConfMedium`, 벤더 폴더 전체) / `[확신낮음]`(`ConfLow`, 이름 기반). `--apply`는 기본적으로 high+medium을 삭제하고 low는 `--include-low` 있을 때만(`Plan.WithoutLowConfidence`가 low만 제거).
   - **패키지 매니저 항목**(`packages.go`): query가 npm 전역/pipx/cargo 패키지거나 `~/go/bin` 실행 파일이면 네이티브 uninstall 명령(`npm uninstall -g`, `pipx uninstall`, `cargo uninstall`) 또는 바이너리 휴지통 이동을 타깃으로 추가.
   - **벤더 폴더 처리**(`vendor.go` + `data/vendors.json` `go:embed`): Google/Microsoft/Adobe/JetBrains 등 벤더 폴더를 만나면 **인벤토리(설치된 앱 Bundle ID)를 참조**해 같은 벤더 다른 앱이 있는지 본다. 있으면 `<Vendor>/<App>` 하위만 `[확신낮음]`으로 추가하고 공유 컴포넌트(Keystone 등)는 `Target.Excluded`에 "타 앱 사용 중" 사유로 보존 표기; 이 벤더 앱이 이것뿐이면 벤더 폴더 전체 + 고아 공유 컴포넌트를 `[확신중간]`으로. 새 벤더 케이스는 **코드 수정 없이 `data/vendors.json`에 추가**. `coveredByExisting`으로 부모 폴더에 중첩된 중복 경로는 제거. brew formula/cask는 네이티브 uninstall 명령 + 역의존성(`brew uses --installed`) 경고 + autoremove 제안, GUI 앱은 Bundle ID 정확 일치 잔여물(높은 확신) + 이름 기반(낮은 확신) 구분. 순수 함수 `IsProtected`(시스템 경로 블랙리스트)와 `IsAppleSystemApp`(`com.apple.*` — Safari처럼 /Applications에 있는 애플 앱 보호)로 안전 강제, `MoveToTrash`는 실행 직전 `IsProtected` 재확인. **아래 안전 규칙 필수 준수.**

## clean 안전 규칙 (마일스톤 5 구현 시 반드시)

- **기본은 dry-run**: 삭제 계획(파일 목록 + 사용할 명령)만 출력. 실제 삭제는 `--apply` + 대화형 확인 필요.
- **rm 금지, 휴지통 이동**: `/usr/bin/trash`(있으면) 또는 `osascript`로 Finder에게 삭제 위임. 사용 가능 여부를 먼저 확인.
- **시스템 경로 하드코딩 블랙리스트**: `/System`, `/usr`, `/bin`, `/sbin`, `/Library/Apple`은 절대 접근 금지.
- **잔여 파일 매칭은 Bundle ID 정확 일치 우선**: `mdfind kMDItemCFBundleIdentifier` 기반. 이름 유사도 기반 매칭 결과는 "확신 낮음"으로 별도 표시.
- **brew 삭제**: `brew uses --installed`로 역의존성 확인 후 경고, 삭제 후 `brew autoremove` 제안.

## 테스트 관례
- 파싱 로직마다 테이블 기반 유닛 테스트. 실제 명령 출력 샘플을 `internal/source/testdata/`에 fixture로 저장하고, 파서 함수에 직접 주입해 검증(exec 미실행).
- 마일스톤 완료 시 프로토타입 JSON과 Go 출력을 구조적으로 diff해 누락 필드가 없는지 확인.

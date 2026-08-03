# macinv

macOS에 설치된 소프트웨어 전체를 스캔해 **"내 맥에 뭐가 어디 깔려있는지"** 파악하고, 업데이트 대기를 표시하고, 잔여물 없이 안전하게 정리하는 Go CLI.

검증된 Python 프로토타입(`mac_inventory.py`)을 Go로 재설계한 것으로, 스캔 로직과 출력 레이어를 분리해 CLI · JSON · TUI를 모두 지원한다.

## 설치

**저장소는 현재 GitHub private**([kyungryeol-yoon/macinv](https://github.com/kyungryeol-yoon/macinv))다. `go install ...@latest`는 공개 모듈 프록시가 private 저장소를 못 읽어와 **동작하지 않는다** — 반드시 clone 후 로컬 빌드/설치한다.

### 이미 clone된 소스에서

```bash
go build -o macinv .   # 현재 디렉토리에 바이너리만
go install .           # ~/go/bin/macinv 로 설치
```

### 새 macOS 기기에서 처음 설정할 때

```bash
brew install go gh              # Go 툴체인 + GitHub CLI (없으면)
gh auth login                   # 최초 1회, 브라우저로 GitHub 로그인
gh repo clone kyungryeol-yoon/macinv
cd macinv
go install .                    # ~/go/bin/macinv 로 설치
```

`gh auth login`으로 인증해두면 git credential helper가 자동 연결되어 `git clone`/`git push`/`go install`이 모두 이 저장소에 접근할 수 있다. (SSH 키를 쓴다면 `git clone git@github.com:kyungryeol-yoon/macinv.git`도 가능.) 저장소를 나중에 public으로 전환하면 맨 위 `go install github.com/kyungryeol-yoon/macinv@latest` 방식도 별도 인증 없이 동작한다.

`~/go/bin`이 PATH에 없다면 셸 프로필(`~/.zshrc`)에 추가한다:

```bash
export PATH="$HOME/go/bin:$PATH"
```

요구사항: **Go 1.25+**(`go.mod`의 `go` 지시자 기준 — `brew install go`로 충족, 이 프로젝트는 1.26.5로 개발), macOS. 스캔 대상 도구(brew/npm/pipx/cargo/mas 등)는 **설치돼 있을 때만** 조회하고, 없으면 조용히 건너뛴다.

설치 확인: `macinv --version` (`-v`) → 현재 `macinv version 0.1.0`.

## 커맨드

| 커맨드 | 설명 |
|---|---|
| `macinv scan` | 전체 인벤토리 스캔 → 한국어 요약. 앱·brew·npm·pipx·cargo·mas·go bin·launchd + **설치된 언어 런타임 버전**까지. `~/.macinv/inventory.json`에 캐시. |
| `macinv outdated` | brew/npm/mas 업데이트 대기 목록 (현재 → 최신). |
| `macinv trace <name>` | 바이너리 소유권 역추적. `which -a`의 각 경로를 심볼릭 링크 따라가 brew/npm/cargo/pipx/go/시스템/수동 중 무엇인지 판별. |
| `macinv runtimes` | node/python/go/rust/java/ruby/php/dotnet/swift의 활성 버전·경로·관리 주체 + 버전 매니저가 관리 중인 버전. **지원 종료(EOL)** 를 endoflife.date로 조회해 `[EOL 지남]`/`[N개월 뒤 종료]`/`[지원 중]`, 업데이트는 `[현재]`/`[안전한 패치 있음]`/`[메이저 최신은 X]`로 표시. 올릴 명령을 **보여주기만** 하고 실행하지 않는다(런타임 메이저는 프로젝트를 깨뜨린다). 7일 캐시, `--offline`로 조회 생략. |
| `macinv clean <name>` | 삭제 플래너. 앱 · brew formula/cask · **npm 전역 · pipx · cargo · go bin** 지원. **기본 dry-run** — 실제 삭제는 `--apply` + 대화형 확인이 있을 때만. |
| `macinv update <name>` | 업데이트 플래너. brew formula/cask · npm 전역 · App Store(mas)를 최신으로. `clean`과 같은 렌더러/실행기 공유. **기본 dry-run**, `--apply`로 실행. **`--all`** 이면 업데이트 대기 전부를 일괄로. |
| `macinv orphans` | **고아 잔여물 역스캔** — 이미 지운 앱들이 `~/Library`에 남긴 데이터를 찾아 정리. Bundle ID 형태 항목만 대상으로 하고, 애플(`com.apple.` 포함·팀ID·systemgroup)·설치 앱 파생물·벤더/공유 프레임워크(Shortcuts, Sparkle)는 보존. **기본 dry-run**, `--apply` + 확인 시 휴지통 이동. |
| `macinv disk` | **디스크를 뭐가 먹고 있는지** — 알려진 캐시/모델/빌드 산출물 위치를 용량순으로. 인벤토리와 대조해 "도구는 없는데 캐시만 남은" **고아 캐시**를 표시하고, APFS 로컬 스냅샷 때문에 지워도 용량이 안 느는 상황을 안내. 정리는 네이티브 명령(`brew cleanup` 등)·휴지통 이동으로 **기본 dry-run**, `--apply` + 확인 시 실행. `--include-expensive`(모델 등), `--deep`(오래된 node_modules 탐색). |
| `macinv tui` | 대화형 TUI. `space` 멀티선택 · `d` 삭제 · `u` 업데이트(계획 모달 → 승인, `[확신낮음]`은 체크로 opt-in, GUI 앱은 y/N 재확인) · `s` 앱 탭 정렬 토글(이름순↔용량순) · **디스크 탭**에서 캐시 위치 선택 후 `d`로 정리 · `r` 재스캔 · `←/→` 탭 · `↑/↓` 이동 · `q` 종료. 실행은 비동기 스트리밍 로그로 표시되고, 완료 후 해당 목록만 자동 갱신. |

공통 플래그:

- `--json` — (scan/outdated/trace/runtimes) 사람용 출력 대신 JSON을 stdout으로.
- `scan --no-outdated` — 네트워크 최신버전 조회 생략(빠름).
- `scan --no-sizes` — 앱 용량(`du`) 계산 생략. 용량은 앱 번들뿐 아니라 `~/Library`의 앱 데이터/캐시까지 재므로, 스캔에서 가장 오래 걸리는 단계다(이 맥 기준 0.9s → 9s).
- `scan --no-runtimes` — 언어 런타임 조회 생략.

### 예시

```bash
macinv scan                 # 전체 스캔 + 요약
macinv scan --json -o out.json
macinv outdated             # 업데이트 대기만
macinv trace git            # git이 어디서 왔는지
macinv runtimes             # 언어별 버전/관리 주체 + 지원 종료(EOL)
macinv runtimes --offline   # 네트워크 조회 없이 캐시만
macinv clean godot          # 삭제 계획만 출력(dry-run)
macinv clean godot --apply  # 확인 후 실제 삭제(휴지통 이동)
macinv update git           # git 업데이트 계획(dry-run)
macinv update git --apply   # 확인 후 실제 업데이트
macinv update --all         # 업데이트 대기 전부 일괄 계획(dry-run)
macinv update --all --apply # 확인 후 전부 업데이트
macinv orphans              # 고아 잔여물(지운 앱이 남긴 데이터) 찾기(dry-run)
macinv orphans --apply      # 확인 후 정리
macinv disk                 # 디스크 사용처 + 고아 캐시 (dry-run)
macinv disk --deep          # 오래된 node_modules까지 탐색
macinv disk --apply         # 확인 후 캐시 정리
macinv tui                  # 대화형 탐색·삭제·업데이트
macinv --version            # 설치된 버전 확인
```

**`--apply` 확인 입력 단어**는 명령마다 다르다 — 실수로 잘못 삭제/업데이트하지 않도록:

- `clean`/`update <name>`: **대상 이름**을 정확히 입력 (예: `godot`)
- `update --all`: **`all`** 입력
- `orphans`: **`orphans`** 입력

정확히 일치하지 않으면 취소되고 아무것도 바뀌지 않는다.

## clean 안전 규칙

삭제는 되돌리기 어려우므로 다음을 코드로 강제한다:

- **기본은 dry-run**: 계획(파일 목록 + 실행할 명령)만 출력. 실제 삭제는 `--apply`와 **대상 이름 정확 입력** 대화형 확인을 모두 통과해야 한다.
- **rm 금지, 휴지통 이동**: `trash` CLI(있으면) 또는 Finder(osascript)로 위임. 삭제 직전 블랙리스트를 한 번 더 확인.
- **시스템 경로 블랙리스트**: `/System`, `/usr`, `/bin`, `/sbin`, `/Library/Apple`은 절대 대상에서 제외. Safari처럼 `/Applications`에 있는 애플 기본 앱(`com.apple.*`)도 보호.
- **확신도 3단계**: `[확실]`(Bundle ID 일치) · `[확신중간]`(벤더 폴더 전체) · `[확신낮음]`(이름 기반). `--apply`는 기본적으로 확실+중간만 삭제하고, 확신낮음은 `--include-low`가 있을 때만 포함.
- **벤더 폴더 스마트 처리**: `~/Library/Application Support/Google` 같은 벤더 공유 폴더는 인벤토리를 참조해, 같은 벤더 다른 앱이 있으면 `<Vendor>/<App>` 하위만 후보로 올리고 Keystone 같은 공유 컴포넌트는 "타 앱 사용 중"으로 보존한다. 그 벤더 앱이 이것뿐이면 폴더 전체를 정리한다. 벤더 매핑은 [`internal/clean/data/vendors.json`](./internal/clean/data/vendors.json)에서 관리(케이스 추가 시 코드 수정 불필요).
- **brew 역의존성 확인**: `brew uses --installed`로 다른 패키지가 의존하는지 경고하고, 삭제 후 `brew autoremove`를 제안.

## 아키텍처

스캔(수집) / 데이터 / 출력을 레이어로 분리한다:

- `internal/source/` — 각 설치 소스가 `Source` 인터페이스를 구현. `errgroup`으로 병렬 스캔. 명령 미설치는 조용한 스킵. 파싱은 exec와 분리된 순수 함수라 fixture로 테스트.
- `internal/exec/` — 모든 외부 명령에 context timeout, 실패는 경고로 수집(`errors` 필드). stdout/stderr 결합 실행(`RunCombined`)으로 java/swift 버전 대응.
- `internal/report/` — JSON 스키마 타입과 한국어 요약 렌더. null 가능 필드는 포인터.
- `internal/clean/` — 삭제(`clean.go`)·업데이트(`upgrade.go`)·고아 잔여물(`orphans.go`) 플래너, 벤더 폴더 처리(`vendor.go`)와 패키지 매니저 삭제(`packages.go`), CLI/TUI 공용 계획 렌더(`render.go`). 안전 규칙은 순수 함수 `IsProtected`/`IsAppleSystemApp`.
- `internal/tui/` — bubbletea 기반 대화형 뷰. 탐색뿐 아니라 `space` 멀티선택·`d` 삭제·`u` 업데이트까지 실행하지만, 실행 로직 자체는 위 `internal/clean`을 호출만 한다(복제 없음).
- `internal/cmd/` — cobra 커맨드(출력 레이어). scan 수집 로직은 `collectInventory`로 tui와 공유.

## 개발

```bash
go build ./...
go test ./...
go vet ./...
go test ./internal/source -run TestParseMas   # 단일 테스트
```

자세한 아키텍처·마일스톤·안전 규칙은 [`CLAUDE.md`](./CLAUDE.md) 참고.

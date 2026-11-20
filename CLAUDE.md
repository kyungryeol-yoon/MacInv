# MacInv

macOS에 설치된 소프트웨어 전체를 스캔해 "내 맥에 뭐가 어디 깔려있는지" 파악하고, 업데이트 대기를 표시하고, 잔여물 없이 안전하게 정리하는 **메뉴 막대 앱**(만드는 중). 사용자 대상 출력은 **한국어**.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 형태 | 메뉴 막대 앱(SwiftUI `MenuBarExtra`). 터미널 CLI는 버린다(2026-11-20) |
| 구현 | **Swift로 완전히 새로 짠다**(2026-11-21). Go 엔진을 호출하지 않는다. 지금 Go 코드는 이식 대상이 아니라 **요구사항·교훈의 출처** — Swift 앱이 같은 일을 하게 되면 지운다 |
| 번들 ID | `io.github.kyungryeol-yoon.macinv` — 개인 앱 공통 규칙 `io.github.kyungryeol-yoon.<앱>`(Hum·DiSwitch와 같음). **바꾸면 설정·권한 허용이 사라지니 고정** |
| 서명 | ad-hoc으로 시작(DiSwitch·Hum과 같음). 업데이트 후 전체 디스크 접근 허용이 풀리는 게 실측되면 고정 자체 서명 인증서로 바꾼다 |
| 배포 | `kyungryeol-yoon/homebrew-tap`의 Cask `macinv`. 틀은 DiSwitch(`packaging/diswitch.rb`, 태그 → Actions → tap 갱신)를 따른다 |
| 라이선스 | MIT(저작권자 「Yoon」, Hum·DiSwitch와 같음) |
| 이름 | 보이는 이름·저장소·Go 모듈은 `MacInv`. 소문자 — Cask 이름 `macinv`(소문자만 허용), 실행 파일 이름. 캐시는 아직 `~/.macinv/`(Swift 앱에선 `~/Library/Application Support/MacInv`가 자연스럽다 — 정할 때 옛 위치는 새로 시작해도 된다, 2026-11-21 비움) |

- GitHub: https://github.com/kyungryeol-yoon/MacInv (public, `main`). 2026-11-20 `macinv`에서 이름 변경
- 커밋·푸시·tap 저장소 변경은 사용자가 요청할 때만. 설명은 한국어
- 앱 아이콘(2026-11-21 정함, 시안 「E2」): **열린 노트북 화면 안에 목록 3줄**(앱 타일 주황·파랑·보라 + 용량 막대) **+ 줄 끝 상태 배지 ↑ 업데이트(파랑) · ✓ 최신(초록) · − 삭제(빨강)** — 확인·업데이트·삭제를 한 그림에. 바탕은 DiSwitch와 같은 어두운 824 둥근 사각형, 테두리에 밝은 선. 모니터 모양은 DiSwitch(모니터 두 대)와 겹쳐서 피했다. **SF Symbol은 앱 아이콘에 쓰지 않는다**(애플 라이선스상 아이콘·로고 사용 금지 — 배지 기호도 도형으로 그림). 그리는 코드는 앱 코드 작업 때 `packaging/make-icon.swift`로 옮긴다 — 시안 코드(`conceptE2`)는 임시 폴더 `/private/tmp/claude-501/-Users-yoon-develop-macos-MacInv/7f287784-3200-4f0f-8e6d-cda82099da97/scratchpad/icons/draft.swift`(재부팅·정리 시 사라질 수 있음 — 없으면 위 설명대로 다시 그린다), 시안 페이지 https://claude.ai/artifact/G4cJcafM9sejmmTz6gUP2h
- 메뉴 막대 아이콘: 미정 — **가장 중요**, Swift 앱 코드 작업하면서 실제 메뉴 막대에 띄워 보고 정한다(후보 `laptopcomputer`·`list.bullet.rectangle`·`shippingbox`. SF Symbol 템플릿. 업데이트·정리할 것이 있으면 채움 모양 또는 점으로 상태 표시)

## 새 구현에서 처음부터 넣을 것

- **자기 자신 보호** — brew로 깔면 MacInv가 cask 목록·`/Applications/MacInv.app`·자기 데이터 폴더·업데이트 대상에 동시에 나타난다. 번들 ID·번들 경로·cask `macinv`로 판정하는 **순수 함수 하나**를 모든 계획 생성에서 확인해 빼고, "지우려면 `brew uninstall --cask macinv`"를 보여준다. 자기 업데이트는 허용하되 **맨 마지막**에, 끝나면 재시작 안내. 실행 중 보호에 우연히 걸리는 것에 기대지 말고 테스트로 고정
- **권한** — 다른 앱의 `~/Library/Containers`·Group Containers·Mail·Safari는 **전체 디스크 접근**, 다른 앱의 `.app` 삭제는 **앱 관리** 권한이 필요하다. 권한이 없으면 용량이 **조용히 적게** 잡히므로 "권한이 없어 일부만 측정"을 화면에 드러낸다. 확인 API가 없어 보호 경로를 실제로 읽어 보고 판정, 없으면 시스템 설정 해당 화면을 여는 버튼. ad-hoc 서명이라 업데이트 후 허용이 풀렸는지도 같은 방법으로 감지해 안내
- **GUI 앱의 PATH** — 셸 환경을 물려받지 않는다(`/usr/bin:/bin:/usr/sbin:/sbin`뿐). `/opt/homebrew/bin/brew`, nvm·mise의 node/npm 위치를 직접 찾는다. sudo가 필요한 cask 업그레이드는 입력할 곳이 없어 멈추므로 계획 단계에서 "수동 처리"로 분리
- **확인** — 대상 이름 입력 대신 GUI 확인 창. `update --all`·orphans·GUI 앱 삭제·expensive 포함 계획은 한 번 더 확인
- **네이티브 API로 대체**
  - 휴지통: `FileManager.trashItem`(Go판의 `/usr/bin/trash` + osascript 폴백과 그 함정이 전부 사라진다)
  - 실행 중 앱: `NSWorkspace.runningApplications`(Bundle ID·번들 경로를 직접 준다 — `ps` 파싱 불필요). 단 **캐시 디렉토리는 앱 이름으로 돼 있어서** 디렉토리 이름 ↔ 앱 매칭 규칙(아래)은 여전히 필요
  - 앱 정보: `Bundle(url:)` / Spotlight. 용량: 파일 시스템 열거(`totalFileAllocatedSize`), **권한 부족 경로를 따로 센다**
- **앱 밖에 깔리는 것·남는 로그인 항목** — 실기 사례(2026-11-21, Riot): 앱을 지웠는데 시스템 설정 「백그라운드 앱 활동」에 「Riot Games」가 남았다. 원인은 `/Users/Shared/Riot Games/Riot Client.app`(1.2 GB, `/Applications`가 아닌 곳에 설치된 본체)과 이를 실행하는 `~/Library/LaunchAgents/com.riot.riotclient.checkinstalls.plist`. Go판은 둘 다 못 잡는다(앱 목록은 `/Applications`만, orphans는 `~/Library`의 Bundle ID 형태만, `/Users/Shared`·LaunchAgents의 **실행 대상이 사라졌거나 지운 앱 소속인 항목**은 안 봄). 새 구현에서: LaunchAgent/Daemon plist의 실행 파일 경로·팀 ID로 소속을 판정해 **"지운 앱이 남긴 백그라운드 항목"**을 보여주고(`sfltool dumpbtm`과 같은 정보), `/Users/Shared` 아래 앱 번들도 앱 목록에 넣는다. 로그인 항목 목록은 plist를 지워도 다음 로그인 때 갱신된다
- **가져갈 데이터**: `internal/paths/data/vendors.json`(벤더 폴더 표), `internal/disk/data/locations.json`(캐시 위치 표) — 코드 수정 없이 항목을 늘리는 구조도 그대로

## 규칙과 교훈 (언어 무관 — 새 구현에서도 테이블 테스트로 고정)

괄호 안은 참고할 Go 위치. 대부분 **실기에서 오탐·데이터 손실 직전까지 가서** 생긴 규칙이라 "편의를 위해 완화"하지 말 것.

### 삭제 안전
- **기본은 계획만**(읽기 전용). 실행은 명시적 승인 뒤에만. **rm 금지, 휴지통으로만.** 휴지통으로 옮긴 건 "휴지통을 비우기 전까지 실제로는 안 빈다"를 계획에 적는다
- **보호 경로**: `/System`, `/usr`, `/bin`, `/sbin`, `/Library/Apple` 접근 금지. `/Applications`의 애플 기본 앱(`com.apple.*`, Safari 등) 보호. 실행 직전 한 번 더 확인 (`paths.IsProtected`/`IsAppleSystemApp`)
- 접두사 블랙리스트는 `~/Library` 아래 애플 데이터를 못 거른다 → 와일드카드 결과는 **경로 마디 단위로** `com.apple.*`를 다시 거른다 (`disk.HasAppleComponent`)
- **확신도 3단계**: [확실] Bundle ID 일치 · [확신중간] 벤더 폴더 전체·고아 잔여물 · [확신낮음] 이름 기반. 기본 실행은 확실+중간, 낮음은 따로 골라야 포함
- **Bundle ID 접두사가 다른 앱을 삼키지 않게**(6-1, 데이터 손실 직전): `com.google.Chrome*`가 `com.google.Chrome.canary`를 [확실]로 잡았다. 경로에서 뽑은 파생 ID가 **설치된 다른 앱에 귀속**되면 빼고 "타 앱(<이름>) 소유 — 보존" 표시. 귀속은 **가장 긴 ID가 이긴다**(둘 다 깔린 상태에서 canary 삭제 시 canary 파생물이 Chrome 것으로 오판되면 안 됨). **어느 앱도 소유하지 않는 파생물(`.helper` 등)은 남겨서 지운다** — 이 구분이 핵심 (`clean.excludeOtherAppIDs`/`ownerOfID`, `TestLeftoversDoNotSwallowOtherApp`)
- **앱 이름은 패턴이 아니다**(6-2): 이름 기반 경로는 리터럴 존재 확인. `"Foo [beta]"`를 glob으로 쓰면 `Foo b`를 삼킨다. 패턴에 끼우는 가변 부분(Bundle ID)은 이스케이프
- **이름 매칭은 정확 일치 우선**, 없을 때만 부분일치 + "이름 부분일치" 경고 (`clean.matchApps`)
- **권한 없는 경로는 미리 분리**(6-10): 부모 디렉토리에 쓰기 권한이 없으면(`/Library/LaunchDaemons/...`, root 소유 `/Applications/Utilities` 아래 앱) "🔑 권한 필요(수동)"로 따로 표시하고 실행·회수 예상에서 뺀다. 실패할 걸 알면서 시도하지 않는다 (`clean/permission.go`)
- **벤더 폴더**(Google·Microsoft·Adobe·JetBrains…): 같은 벤더의 다른 앱이 설치돼 있으면 `<Vendor>/<App>`만 [확신낮음]으로, 공유 컴포넌트(Keystone 등)는 "타 앱 사용 중"으로 보존. 그 벤더 앱이 마지막이면 폴더 전체+고아 공유 컴포넌트를 [확신중간]으로 (`clean/vendor.go`, `vendors.json`)
- 잔여물은 Bundle ID 접두사로 헬퍼·파생 파일(`.ShipIt`, `.helper.plist`)·Group Containers·Cookies·ByHost·`/Library`까지 찾는다. 부모에 이미 포함된 경로는 뺀다 (`paths.gatherLeftovers`)
- **brew**: 네이티브 `brew uninstall`, `brew uses --installed`로 역의존성 경고, 끝나면 `brew autoremove` 제안. npm 전역·pipx·cargo도 네이티브 uninstall, `~/go/bin` 실행 파일은 휴지통 (`clean/packages.go`)
- **명령은 인자 배열로 저장**(6-8): 문자열로 저장했다 다시 쪼개면 `--filter "until=24h"`가 깨진다. 표시용 한 줄은 렌더할 때만 만들고 다시 파싱하지 않는다
- **실행(변경) 명령은 엄격 판정**: 종료 코드로 성공을 본다(부분 실패를 성공으로 오인 금지). 실패 메시지에 stderr 포함. 업그레이드는 타임아웃 30분, 삭제는 5분

### 고아 잔여물 (지운 앱이 `~/Library`에 남긴 것)
- Bundle ID 형태(점 2개 이상)만 대상. `/Library`는 안 본다(다른 사용자·서비스 소유 가능). 전부 [확신중간]
- **보존**(실기 오탐 — Apple Podcasts 그룹·단축어·Sparkle): `com.apple.`이 **어디든 포함**, `systemgroup.`, 팀ID 접두사 `^[A-Z0-9]{10}\.`, `is.workflow.`, `org.sparkle-project.`, 설치 앱 ID와 **양방향** 접두사 관계, 벤더 공유 (`clean.classifyOrphan`/`orphanProtected`)

### 용량 측정
- **앱 용량 = 번들 + 데이터.** 번들만 재면 "번들 283 MB, 캐시 67 GB"인 Electron 앱이 안 보인다. 표시는 내역과 함께(`번들 2.0 GB + 데이터 8.8 GB`) — 번들은 앱을 지워야 줄고, 데이터는 정리로 줄 수 있다는 신호
- 측정 경로는 삭제 규칙에 두 가지를 **더한 넓은 규칙**이고 **삭제에는 쓰지 않는다**: ① 벤더 폴더 안 앱 하위 폴더(Chrome 데이터는 `Application Support/Google/Chrome` — ID로도 이름으로도 안 잡힘) ② 그룹 컨테이너 이름 매칭 — 팀ID를 떼고 **마지막 마디만 정확 비교**(OrbStack은 ID가 `dev.kdrag0n.MacVirt`라 `HUAQ24HBR6.dev.orbstack` 24 GB를 ID로는 못 찾는다). 부분일치 금지("Mail"/"Home"이 무관한 컨테이너를 쓸어담음) (`paths.SizePaths`/`groupContainerMatchesApp`)
- **표시 용량 ≠ 회수 가능량**(6-5): "Chrome 10.9 GB"를 지웠는데 2 GB만 비는 게 정상이다(벤더 공유 보존). 계획에는 **실제로 담긴 경로만** 합한 "💾 이번 삭제로 회수 예상"을 따로 — [확신낮음] 제외 기준 + 모두 포함 상한을 괄호로, 체크 상태가 바뀌면 즉시 재계산, 권한 필요 경로는 제외 (`clean/reclaim.go`)
- **부모·자식 경로를 같이 재면 두 번 센다**(6-4): 양방향으로 합치고, 부모가 나중에 들어오면 기존 자식을 뺀다 (`paths.DedupContained`)
- **경로마다 제 용량을 따로 갖는다**: 위치 합계를 경로마다 넣으면 경로 N개일 때 회수 예상이 N배(Electron 캐시 9경로에서 1 GB→9 GB로 드러남) (`disk.Entry.PathSizes`)
- 앱 메타데이터는 앱당 1회 조회, **키 기반 파싱** — 위치 기반 출력은 순서가 뒤바뀌어 version/date_added가 서로 바뀌었다(Python 프로토타입 버그)
- 측정이 오래 걸리면(수십 GB 캐시 트리, 이 맥 9초) 진행률을 보여준다(6-6)

### 디스크 정리 (원래 동기: 시스템 설정 "문서" 57 GB, Finder로는 11 GB — `~/.ollama` 모델과 개발 캐시)
- 알려진 위치는 **데이터**(`locations.json`: id/name/globs/tool/tier/cleanup/trash/note). 네이티브 정리 명령이 없으면 `note`로 이유를 반드시 남긴다(막다른 길 금지 — 테스트로 강제)
- **등급은 "지우면 무엇을 잃는가"**: `safe`(재생성) / `artifact`(빌드 산출물) / `expensive`(재다운로드 비쌈) / `never`(소스·사용자 데이터). 기본은 safe+artifact, expensive는 옵트인, **never는 어떤 옵션으로도 안 뚫린다** (`disk.Cleanable`)
- 네이티브 명령(`brew cleanup` 등)이 있으면 그걸, 없으면 휴지통. 회수 예상에는 **명령이 비울 몫도** 넣는다(안 그러면 요약 3.1 GB vs 계획 139 MB로 어긋남)
- **와일드카드 위치는 앱별로 쪼개 보여준다** — "66 GB" 한 줄로는 어느 앱을 손볼지 모른다. 상위 5개 + "외 N개" (`disk.GroupUsage`/`BreakdownLine`)
- **고아 캐시**(캐시는 있는데 쓰는 도구가 없음) — 인벤토리를 가진 이 도구만 할 수 있는 판단이라 따로 보여준다. **오탐 방지 3중 장치, 하나라도 빼면 멀쩡한 캐시가 고아로 뜬다**: ① 인벤토리(brew/npm/pipx/cargo/gobin/앱/런타임 이름 + Bundle ID 마지막 마디) ② PATH에서 실행 파일 직접 확인(인벤토리가 낡아 python이 없다고 보고 pip 캐시를 오탐한 실측; pip→pip3, simctl→xcrun 같은 이름 차이 포함) ③ `owner_apps` — CLI 없이 GUI 앱만 쓰는 경우(Android Studio→`~/.gradle`, IntelliJ→`~/.m2`). 이름 정규화는 **넣을 때와 찾을 때 같은 함수**("Android Studio"→`android studio`/`androidstudio`). 별칭(pip←python)도 완화 금지 (`disk/orphan.go`)
- **Ollama는 모델별로** 보여주고 개별 `ollama rm` — 위치 통째로 지우지 않는다
- **APFS 로컬 스냅샷**은 "지워도 여유 공간이 안 느는 이유"로 **안내만** 한다. 스냅샷 삭제는 백업 복원 지점을 없애는 일이라 도구가 결정하지 않는다
- 오래된 `node_modules` 찾기는 별도 옵션(깊이 6, 숨김·`Library`·미디어·클라우드 동기화 폴더 제외, **6개월 이상 안 바뀐 것만**, 예산 120초). 기본 스캔은 알려진 위치만(이 맥 1.3초)
- **Electron 앱 캐시**(Notion 66 GB 사례): 앱을 하드코딩하지 않고 `~/Library/Application Support/*/` 아래 `Cache`·`Code Cache`·`GPUCache`·`Service Worker/CacheStorage`·`Service Worker/ScriptCache`. **`Partitions/<파티션>/` 아래도 같은 구조**(Notion·Slack 워크스페이스, Claude preview). 앱별 합계는 파티션이 아니라 **앱 단위로** 묶는다
  - **`IndexedDB`·`Local Storage`·`Cookies`·`Session Storage`는 절대 포함 금지**(Partitions 아래도) — 로그인 세션·로컬 데이터라 지우면 캐시 정리가 아니라 데이터 삭제 (`TestElectronLocationNeverTargetsUserData`)
  - 앱 단위 삭제 계획과 경로가 겹치면 넓은 쪽 하나만 (`TestElectronPathsNeverNest`)
- **실행 중인 앱의 캐시는 지우지 않는다** — Chromium은 캐시 파일을 연 채로 돈다(켜둔 채 지우면 오작동). 기본 제외 + "실행 중(<앱>) — 종료 후 다시". 요약에 "⏸ N개 앱 실행 중이라 제외 (총 X GB)"와 앱별 내역
  - 디렉토리 이름 ↔ 앱 매칭: 정규화 완전 일치 → 별칭 표 → 앱 이름을 **단어로** 쪼갰을 때 한 단어로 들어 있음("Code" ⊂ "Visual Studio Code"). **부분 문자열 금지**("Home"/"Mail"). 애매하면 보호 쪽으로. 헬퍼가 중첩된 번들은 전부 실행 중으로 (`disk.MatchesRunningApp`)
  - **조회 실패 ≠ 실행 중인 앱 없음.** 예전엔 실패 시 보호가 **조용히 꺼졌다** — 안전장치가 조용히 사라지는 건 처음부터 없는 것보다 나쁘다. 실패했고 Electron 캐시가 계획에 담겼을 때만 계획 맨 위에 경고
  - UI에서는 이 보호를 끄는 수단을 두지 않는다

### 업데이트
- brew formula/cask, npm 전역(`npm install -g <pkg>@latest`), App Store(`mas upgrade <id>`). 삭제와 같은 계획/실행/렌더를 쓴다
- **npm 자기 업데이트는 `npx -y npm@latest install -g npm@latest`** — 실행 중인 npm이 자기 모듈을 지우다 MODULE_NOT_FOUND로 죽는다(실측)
- **"전부 업데이트"에 런타임 메이저를 넣지 않는다** — brew는 node 25→26도 그냥 outdated로 보고하는데, 생각 없이 누르는 버튼에 프로젝트를 깨뜨릴 것이 들어가면 안 된다. "⏸ 제외" 목록과 명령만 보여준다. 같은 사이클 패치는 포함. 개별로 고른 건 경고와 함께 실행 (`clean.SplitRuntimeMajors`)

### 런타임 (node/python/go/rust/java/ruby/php/dotnet/swift)
- 목적은 **최신 강요가 아니라 판단 근거** — 몇 달 만에 돌아왔을 때 "깔려 있나·몇 버전·올려야 하나"
- 활성 버전·경로·관리 주체(nvm/pyenv/mise/asdf/rustup/sdkman 경로 우선 → brew/npm/pipx/cargo/system/수동). 실제 경로가 2개 이상 다르면 ⚠️ 충돌. 버전 출력이 stderr로 나오는 것(java/swift) 포함, swift는 `swift-driver` 버전 오인 주의 (`source/runtimes.go`, `trace.go`)
- **EOL**: endoflife.date. java는 `java`/`openjdk` 슬러그가 없어서(404) `eclipse-temurin`. dotnet·swift 제외. 7일 캐시, **실패·오프라인은 에러가 아니라 생략**, 자주 도는 화면은 캐시만 (`source/eol.go`)
- **사이클 매칭은 가장 긴 접두사**("1.2"가 "1.26.5"를 집으면 안 됨). 업데이트 상태는 세 갈래 `[현재]`/`[안전한 패치 있음]`/`[메이저 최신은 X]` — 사이클 안 최신 패치와 최신 사이클을 따로 둔다
- "메이저"는 언어마다 다르다: node/openjdk/deno/bun 1마디, python/ruby/php 2마디. **go/rust는 1.x 안에서 경고하지 않는다 — 의도된 결정, 뒤집지 말 것**(1.x 하위 호환이 정책. 릴리스마다 경고가 뜨면 진짜 위험한 node 경고까지 무시하게 된다). `node@22` 같은 고정 포뮬러도 제외
- **런타임은 실행 대상이 아니다** — 권장 명령을 보여주기만(시스템 런타임은 "따로 설치": mise 있으면 mise, 없으면 brew)

### 외부 명령 (읽기)
- 모든 호출에 타임아웃. 도구 미설치는 에러가 아니라 **조용히 건너뜀**
- 읽기 명령은 종료 코드가 0이 아니어도 stdout이 있으면 쓴다(`npm ls`/`npm outdated`는 정상에도 exit 1). **단 권한 오류로 일부만 읽힌 결과는 "일부"로 표시** — Go판은 이걸 구분하지 못해 용량이 조용히 적게 잡힐 수 있었다

### UI 교훈 (2026-11-21 삭제한 TUI에서)
- **목록의 기본 동작(enter/더블클릭)이 삭제를 시작하면 안 된다** — 승인도 같은 키라 두 번 누르면 실제로 지워졌다(6-7). 삭제·업데이트는 명시적 버튼으로만
- **선택 대상의 종류를 섞지 않는다** — 캐시 위치 ID와 앱/패키지 이름이 섞이면 앱 삭제 계획에 캐시 ID가 들어간다
- **섹션 숫자는 실제 데이터 개수로** — "(없음)"·"✅ 최신 상태" 안내 행을 세서 "업데이트 (1)"이 뜬 버그가 있었다
- **선택은 행 위치가 아니라 대상 이름 기준**(정렬해도 유지). 정렬은 공유 데이터의 사본에
- expensive 항목은 "직접 골랐다"가 옵트인이지만 GUI 앱 삭제와 같은 재확인. [확신낮음]은 기본 체크 해제
- 런타임 행은 삭제/업데이트 불가. 실행은 비동기·한 번에 1개·줄 단위 로그·실패 건수 요약, 끝나면 영향받은 소스만 다시 스캔

## 현재 Go 구현 (참고용 — Swift 완성 후 삭제)

Go 1.25+(개발은 brew go 1.26.5), 의존성 `spf13/cobra`·`golang.org/x/sync`. 모듈 `github.com/kyungryeol-yoon/MacInv`.

```bash
go build ./... && go vet ./... && go test ./...
go run . scan            # scan · outdated · trace <name> · runtimes · disk · clean <name> · update <name>|--all · orphans
go run . scan --json     # scan/outdated/trace/runtimes/disk는 --json 지원
```
`clean`·`update`·`orphans`·`disk`는 기본 dry-run, `--apply` + 터미널 확인 입력(대상 이름/`all`/`orphans`/`disk`)으로 실행. 결과 캐시는 `~/.macinv/inventory.json`, `eol-cache.json`.

의존은 아래로만: `cmd → disk → clean → paths → exec`, `source → paths`, `report`는 순수 데이터. **경로 판별·매칭 규칙은 paths에 한 벌만** — 측정과 삭제가 다른 규칙을 갖는 순간 "보이는 용량"과 "지워지는 것"이 어긋난다(Swift에서도 같은 원칙).

| 패키지 | 내용 |
|---|---|
| `internal/paths` | 보호 경로, 잔여물 매칭, 측정 경로, 벤더 표(`data/vendors.json`), Bundle ID 유틸, 중복 경로 제거 |
| `internal/source` | 소스별 스캔(apps/brew/npm/pipx/cargo/mas/gobin/launchd, 병렬). 파서는 순수 함수 + `testdata/` fixture. runtimes·EOL·trace |
| `internal/clean` | 삭제·업데이트·고아 잔여물 계획(`Plan`/`Target`), 실행(`Execute`), 렌더(`PlanLines`/`RenderPlanText`), 권한·회수 예상 |
| `internal/disk` | 캐시 위치(`data/locations.json`), 등급, 고아 캐시, Electron·실행 중 보호, Ollama, 스냅샷, deep |
| `internal/report` | JSON 스키마 타입, 한국어 요약, 런타임 배지 |
| `internal/exec` | 외부 명령 실행기(타임아웃, 경고 수집, 엄격 모드) |
| `internal/cmd` | cobra 명령 |

부르는 곳이 없는 함수: `clean.BuildCombinedPlan`/`BuildCombinedUpgradePlan`(여러 개 선택용, TUI가 쓰던 것), `exec.RunJSON`, `disk.BuildOllamaPlan`, `disk.SumDuKB`, `source.extractVersion`.

## 테스트 관례
- **안전 판정은 전부 순수 함수 + 테이블 테스트**(파일 시스템·외부 명령 없이). 파일 시스템이 꼭 필요하면 임시 폴더에 실제 파일을 만들어서
- 외부 명령 파서는 실제 출력 샘플을 fixture로 저장해 검증(Go: `internal/source/testdata/`)
- **실기 오탐이 나오면 반드시 회귀 테스트로 고정**
- 동작이 바뀌면 안 되는 변경은 전후 출력을 diff로 비교(용량·생성 시각은 제외). Swift 구현은 같은 맥에서 Go판 `--json` 출력과 대조해 빠진 것이 없는지 본다

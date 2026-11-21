# MacInv

macOS에 설치된 소프트웨어 전체를 스캔해 "내 맥에 뭐가 어디 깔려있는지" 파악하고, 업데이트 대기를 표시하고, 잔여물 없이 안전하게 정리하는 **메뉴 막대 앱**(만드는 중). 사용자 대상 출력은 **한국어**.

## 결정 사항

| 항목 | 결정 |
|---|---|
| 형태 | 메뉴 막대 앱. 터미널 CLI는 버린다(2026-11-20). 메뉴 막대는 AppKit `NSStatusItem`(Hum과 같음) — 왼쪽 클릭 카드 패널(팝오버), **오른쪽 클릭 짧은 메뉴**(열기·다시 스캔·끝내기). SwiftUI `MenuBarExtra`는 오른쪽 클릭을 못 받아 바꿨다(2026-11-21 사용자 요청 — 무심코 오른쪽 클릭하는 습관). 목록 창도 AppKit `NSWindow` + `NSHostingController`(`sceneBridgingOptions = .all`로 툴바·검색창 연결) |
| 구현 | **Swift로 완전히 새로 짠다**(2026-11-21). Go 엔진을 호출하지 않는다. 지금 Go 코드는 이식 대상이 아니라 **요구사항·교훈의 출처** — Swift 앱이 같은 일을 하게 되면 지운다 |
| 번들 ID | `io.github.kyungryeol-yoon.macinv` — 개인 앱 공통 규칙 `io.github.kyungryeol-yoon.<앱>`(Hum·DiSwitch와 같음). **바꾸면 설정·권한 허용이 사라지니 고정** |
| 서명 | ad-hoc(DiSwitch·Hum과 같음, 인증서 없음 — 2026-11-21 다시 확인하고 유지). **실측: 권한 허용은 cdhash에 묶여 다시 빌드하면 풀린다**(설정 목록엔 켜져 있어도 적용 안 됨), 그리고 **켠 뒤 다시 시작해야 적용된다**. 앱은 메뉴를 열 때마다 다시 확인하고 "다시 시작"·"지웠다 다시 추가" 안내를 띄운다. 개발 중엔 터미널에 전체 디스크 접근을 주지 않는다(그 안의 모든 명령이 권한을 갖게 되므로) — 권한이 필요한 시험 때만 설정에서 MacInv를 지웠다 다시 추가. 배포 후 업데이트마다 다시 켜는 게 번거로우면 7단계에서 고정 자체 서명 인증서(로그인 키체인, `.p12` 백업, Actions Secret)로 바꾼다 |
| 권한 원칙 | **권한 없이도 동작한다**(목록·업데이트·대부분의 용량). 전체 디스크 접근은 "더 정확하게(보호 폴더 용량)·보호 폴더 정리"가 필요할 때만 — 조르지 않고, 필요한 곳(⚠️ 일부만 측정, 6단계 계획의 보호 폴더)에서만 이유와 켜는 방법을 보여 준다(2026-11-21 사용자 확인) |
| 배포 | `kyungryeol-yoon/homebrew-tap`의 Cask `macinv`. 틀은 DiSwitch(`packaging/diswitch.rb`, 태그 → Actions → tap 갱신)를 따른다 |
| 라이선스 | MIT(저작권자 「Yoon」, Hum·DiSwitch와 같음) |
| 이름 | 보이는 이름·저장소·Go 모듈은 `MacInv`. 소문자 — Cask 이름 `macinv`(소문자만 허용), 실행 파일 이름. 캐시는 아직 `~/.macinv/`(Swift 앱에선 `~/Library/Application Support/MacInv`가 자연스럽다 — 정할 때 옛 위치는 새로 시작해도 된다, 2026-11-21 비움) |

- GitHub: https://github.com/kyungryeol-yoon/MacInv (public, `main`). 2026-11-20 `macinv`에서 이름 변경
- 커밋·푸시·tap 저장소 변경은 사용자가 요청할 때만. 설명은 한국어
- 앱 아이콘(2026-11-21 정함, 시안 「F1」): **밝은 목록 카드 하나에 3줄**(앱 타일 주황·파랑·보라 + 용량 막대) **+ 줄 끝 상태 배지 ↑ 업데이트(파랑) · ✓ 최신(초록) · − 삭제(빨강)** — 확인·업데이트·삭제를 한 그림에, 메뉴 막대 `checklist`와 같은 "목록 + 줄 끝 상태". 바탕은 DiSwitch와 같은 어두운 824 둥근 사각형. 노트북 안에 넣은 안(E2)은 화면이 작아져 뺐고, 모니터 모양은 DiSwitch와 겹친다. **SF Symbol은 앱 아이콘에 쓰지 않는다**(애플 라이선스상 아이콘·로고 사용 금지 — 배지 기호도 도형으로 그림). 그리는 코드 `packaging/make-icon.swift` → `packaging/AppIcon.icns`. 시안 비교 페이지 https://claude.ai/artifact/G4cJcafM9sejmmTz6gUP2h
- 메뉴 막대 아이콘(2026-11-21 정함): SF Symbol **`checklist`**(템플릿) — 노트북(`laptopcomputer`)은 메뉴 막대에서 눈에 안 띄어 뺐다. 업데이트·정리할 것이 있으면 점 등으로 상태 표시(방식은 3단계에서)

## Swift 구현 (만드는 중)

```bash
swift test                 # MacInvCore 테스트(Swift Testing)
./build.sh                 # build/MacInv.app + zip (ad-hoc 서명)
open build/MacInv.app      # 설치 없이 실행 · ./build.sh install 이면 /Applications에 설치
```

- `Sources/MacInvCore/` — 로직(화면 없음, 테스트 대상). `Command.swift`(실행기: 타임아웃, 미설치=`notFound`, 읽기는 stdout 있으면 성공·`strict`는 종료 코드로만), `SearchPath.swift`(GUI 앱 PATH = 로그인 셸 `-ilc`에서 표식으로 받아온 PATH → 알려진 위치), `Permissions.swift`(전체 디스크 접근 — 보호 경로를 실제로 열어 판정, 안내 판정 `notice`는 순수 함수), `SelfIdentity.swift`(자기 자신 판정: 번들 ID·번들 경로·`/Applications/MacInv.app`·데이터 경로·cask `macinv`)
- `Sources/MacInv/` — 메뉴 막대 화면(SwiftUI)
- `Tests/MacInvCoreTests/`
- 진행: 0단계(뼈대·아이콘) ✅ · 1단계(기반) ✅ · 2단계(인벤토리: 앱·brew·패키지·런타임·백그라운드 항목·앱 데이터 용량) ✅ · 3단계(업데이트 대기·런타임 EOL, EOL 캐시는 `~/Library/Caches/<번들 ID>/eol-cache.json`) ✅ · 4단계(디스크: 위치·고아 캐시·실행 중 보호·심볼릭 링크/외장 볼륨·스냅샷·Ollama 모델, 표 정렬·분류·🔒) ✅ · 5-1(삭제 계획: 앱·brew·패키지, `Plan.swift`·`Planner.swift`, 계획 시트) ✅ · 5-2(업데이트·고아 잔여물·디스크·Ollama 계획) ✅ · 6(실행: `Executor.swift`, 확인 화면 → 로그 → 결과) ✅ · 다음 7 배포 → 6 실행 → 7 배포. 단계마다 이 맥에서 실행해 확인받고, Go판 `--json`과 대조
- 화면: 메뉴 막대는 요약·바로가기(창 열기·다시 스캔·권한 안내 — 단계마다 "손댈 게 있나"를 늘려 간다), 목록은 별도 창(왼쪽 분류 + 표)
- `Paths.swift`는 측정(`sizePaths`)과 삭제 계획(`leftovers`, 5단계)이 **같이 쓰는 한 벌**. glob을 쓰지 않고 폴더 목록을 문자열로 비교(이름의 `[`·`*`·`?`가 패턴이 될 수 없음). 벤더 표는 `Vendors.swift`의 JSON 문자열(SwiftPM 리소스 번들은 .app 서명과 안 맞음)
- **실행기 교훈(2026-11-21 실측)**: `Process.waitUntilExit()` 금지 — 시작한 스레드의 런루프로 종료를 받는데 `await` 뒤 다른 스레드에서 이어지면 영영 못 받고 멈춘다(앱 6개 동시 측정이 10분 넘게 멈춤). 종료는 `terminationHandler`(run() 전에), 블로킹 파이프 읽기는 GCD 스레드에서(동시성 풀을 막으면 동시 실행이 많을 때 전체가 멈춤). 회귀 테스트: 40개 동시 실행. **타임아웃은 프로세스 그룹째**(2026-11-21 점검): 바로 아래 프로세스만 끄면 그 하위 프로세스(brew → ruby·curl, `sh -c 'x &'`)가 파이프를 잡고 있어 읽기가 안 끝났다(실측 30초 멈춤). `posix_spawn`으로 새 그룹에서 시작 → 타임아웃 때 `killpg` SIGTERM → 2초 뒤 SIGKILL → 3초 뒤에도 안 닫히면 읽기를 포기하고 돌아온다. stdin은 /dev/null(sudo 등 대화형 입력은 바로 실패)
- 용량이 권한 때문에 덜 잡히면 `dataPartial` + `deniedPaths`(잰 폴더 단위)로 "일부만 측정 · 읽지 못한 곳"을 보여 준다
- 데이터 파일(`vendors.json`·`locations.json`)은 쓰는 단계(2·4)에서 리소스로 가져온다 — 그때 `build.sh`가 SwiftPM 리소스 번들을 앱에 복사하게 고칠 것

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
- **실행 직전 마지막 관문**(`Executor.refuse`): 보호 경로·MacInv 자신·**통째로 옮기면 안 되는 폴더**(홈, ~/Library와 그 주요 하위·Containers·Group Containers, Documents·Desktop·Downloads·Pictures·Movies·Music, ~/Applications, /Applications, /Volumes/<디스크> 자체 등 — 대소문자 무시)는 계획에 들어 있어도 막는다. 지금 계획에서 나오지 않는 경로라도 안전장치로 둔다(2026-11-21 점검)
- **보호 경로**: `/System`, `/usr`, `/bin`, `/sbin`, `/Library/Apple` 접근 금지. `/Applications`의 애플 기본 앱(`com.apple.*`, Safari 등) 보호. 실행 직전 한 번 더 확인 (`paths.IsProtected`/`IsAppleSystemApp`)
- 접두사 블랙리스트는 `~/Library` 아래 애플 데이터를 못 거른다 → 와일드카드 결과는 **경로 마디 단위로** `com.apple.*`를 다시 거른다 (`disk.HasAppleComponent`)
- **확신도 3단계**: [확실] Bundle ID 일치 · [확신중간] 벤더 폴더 전체·고아 잔여물 · [확신낮음] 이름 기반. 기본 실행은 확실+중간, 낮음은 따로 골라야 포함
- **Bundle ID 접두사가 다른 앱을 삼키지 않게**(6-1, 데이터 손실 직전): `com.google.Chrome*`가 `com.google.Chrome.canary`를 [확실]로 잡았다. 경로에서 뽑은 파생 ID가 **설치된 다른 앱에 귀속**되면 빼고 "타 앱(<이름>) 소유 — 보존" 표시. 귀속은 **가장 긴 ID가 이긴다**(둘 다 깔린 상태에서 canary 삭제 시 canary 파생물이 Chrome 것으로 오판되면 안 됨). **어느 앱도 소유하지 않는 파생물(`.helper` 등)은 남겨서 지운다** — 이 구분이 핵심 (`clean.excludeOtherAppIDs`/`ownerOfID`, `TestLeftoversDoNotSwallowOtherApp`)
- **Bundle ID 접두사는 마디 경계까지만**(2026-11-21 점검): `com.foo.Bar`는 `com.foo.Bar`·`com.foo.Bar.*`만 잡고 `com.foo.BarTender`는 안 잡는다. 그룹 컨테이너도 ID 앞뒤가 마디 경계일 때만. 설치 안 된 다른 앱·앱 없는 도구의 데이터는 `excludeOtherApps`로 걸러지지 않으므로 매칭 단계에서 막아야 한다 (`Paths.matches`, `leftoversRespectSegmentBoundary`)
- **앱 이름은 패턴이 아니다**(6-2): 이름 기반 경로는 리터럴 존재 확인. `"Foo [beta]"`를 glob으로 쓰면 `Foo b`를 삼킨다. 패턴에 끼우는 가변 부분(Bundle ID)은 이스케이프
- **이름 매칭은 정확 일치 우선**, 없을 때만 부분일치 + "이름 부분일치" 경고 (`clean.matchApps`)
- **못 옮기는 이유는 두 가지로 나눈다**(2026-11-21 실측): 내 소유인데 쓰기가 막힌 것(`Claude.app`·`Chrome.app` 같은 다른 개발자 앱 번들 = macOS 「앱 관리」 보호, 다른 앱 데이터 폴더 = 앱 데이터 보호)은 **관리자 암호로 안 풀리고** MacInv에 권한을 줘야 한다 → 🛡 + 설정 열기. root 소유(pkg로 깐 앱·`/Library/LaunchDaemons`)는 🔑 관리자 필요. 판정은 MacInv 프로세스 기준이라 터미널에서 잰 결과와 다를 수 있다 (`Planner.trashAccess`)
- **파생 앱은 벤더의 다른 앱이 아니다**: Chrome 웹 앱(`com.google.Chrome.app.…`, `~/Applications/Chrome Apps.localized`)을 "다른 Google 앱"으로 세면 Chrome 계획이 Google 폴더를 공유 중으로 오판한다(실측)
- **권한 없는 경로는 미리 분리**(6-10): 부모 디렉토리에 쓰기 권한이 없으면(`/Library/LaunchDaemons/...`, root 소유 `/Applications/Utilities` 아래 앱) "🔑 권한 필요(수동)"로 따로 표시하고 실행·회수 예상에서 뺀다. 실패할 걸 알면서 시도하지 않는다 (`clean/permission.go`)
- **벤더 폴더**(Google·Microsoft·Adobe·JetBrains…): 같은 벤더의 다른 앱이 설치돼 있으면 `<Vendor>/<App>`만 [확신낮음]으로, 공유 컴포넌트(Keystone 등)는 "타 앱 사용 중"으로 보존. 그 벤더 앱이 마지막이면 폴더 전체+고아 공유 컴포넌트를 [확신중간]으로 (`clean/vendor.go`, `vendors.json`)
- 잔여물은 Bundle ID 접두사로 헬퍼·파생 파일(`.ShipIt`, `.helper.plist`)·Group Containers·Cookies·ByHost·`/Library`까지 찾는다. 부모에 이미 포함된 경로는 뺀다 (`paths.gatherLeftovers`)
- **brew**: 네이티브 `brew uninstall`, `brew uses --installed`로 역의존성 경고, 끝나면 `brew autoremove` 제안. npm 전역·pipx·cargo도 네이티브 uninstall, `~/go/bin` 실행 파일은 휴지통 (`clean/packages.go`)
- **sudo가 필요한 cask**(artifacts에 `pkg`·`installer`)는 「전부 업데이트」에서 빼고 명령만 보여 준다(GUI엔 암호 입력할 곳이 없어 멈춤) — `BrewCask.usesInstaller`
- **고아 캐시**(쓰는 도구가 없음)는 그 도구의 정리 명령이 못 돌므로 폴더를 휴지통으로. 바로가기(심볼릭 링크) 경로는 휴지통에 넣지 않는다(링크만 옮겨지고 공간은 안 빔)
- **명령은 인자 배열로 저장**(6-8): 문자열로 저장했다 다시 쪼개면 `--filter "until=24h"`가 깨진다. 표시용 한 줄은 렌더할 때만 만들고 다시 파싱하지 않는다
- **실행(변경) 명령은 엄격 판정**: 종료 코드로 성공을 본다(부분 실패를 성공으로 오인 금지). 실패 메시지에 stderr 포함. 업그레이드는 타임아웃 30분, 삭제는 5분

### 고아 잔여물 (지운 앱이 `~/Library`에 남긴 것)
- Bundle ID 형태(점 2개 이상)만 대상. `/Library`는 안 본다(다른 사용자·서비스 소유 가능). 전부 [확신중간]
- **도구·같은 개발자도 보존**(2026-11-21 Swift판 실측 오탐): ID 마디 하나가 설치된 포뮬러·패키지·런타임 이름이면 보존(`org.swift.swiftpm`←swift, `sh.brew.ollama`←brew services), 설치 앱과 같은 개발자(앞 두 마디, `io.github`류는 세 마디)면 보존(`com.logi.cp-dev-mgr`). 대소문자 무시. 파생 ID는 상위 고아 한 줄로 묶는다 (`Planner.isOrphan`)
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
- **실행 중인 앱은 지우지 않는다 — cask도 마찬가지**(2026-11-21 점검): `uninstall quit:`이 없는 cask는 brew가 앱을 끄지 않고 번들을 지운다. 계획이 cask의 앱 번들 경로(`appBundles`)를 담고 실행 직전에 확인해 건너뛴다. 업그레이드는 막지 않고 경고만
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
- **표(Table) 칸 안에서는 `.help` 툴팁이 안 뜬다**(2026-11-21 실측) → `InfoBadge`(⚠️ 아이콘에 마우스를 올리거나 누르면 팝오버). 팝오버 안 `Text`는 표 칸의 한 줄 제한을 물려받아 잘리므로 `.lineLimit(nil)`
- **표는 모든 열이 정렬되고(SwiftUI `Table`은 Comparable 키패스가 있어야 정렬됨 — 옵셔널은 `orZero`/`orDash`로) 위에 분류 버튼(개수 포함)**. 건드리면 안 되는 것(애플 앱·시스템 런타임·다른 포뮬러가 쓰는 포뮬러·MacInv 자신·애플 백그라운드)은 🔒 + 이유(2026-11-21 사용자 요청)
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

# MacInv

**내 맥에 뭐가 어디 깔려 있는지** 한눈에 보고, 업데이트할 것을 알려 주고, 지울 때는 잔여물까지 안전하게 정리하는 macOS 메뉴 막대 앱.

> **만드는 중입니다.** SwiftUI 메뉴 막대 앱으로 새로 만들고 있고, 완성되면 Homebrew로 배포합니다.
> 지금 저장소에 있는 Go 코드는 먼저 만들어 이 맥에서 검증한 명령줄 버전으로, 새 앱의 참고용입니다.

## 하는 일

- **인벤토리** — 앱, Homebrew(formula·cask), npm 전역·pipx·cargo·Go 실행 파일, App Store 앱, launchd 항목, 언어 런타임(node·python·go·rust·java…)을 한 번에 모아 봅니다. 앱 용량은 번들뿐 아니라 `~/Library`의 데이터·캐시까지 합쳐서 보여 줍니다.
- **업데이트** — brew·npm·App Store의 업데이트 대기 목록. 런타임은 지원 종료(EOL)와 "안전한 패치 / 메이저 업그레이드"를 구분해 보여 줍니다.
- **디스크** — 디스크를 무엇이 차지하고 있는지(개발 캐시·모델·빌드 산출물·Electron 앱 캐시). 도구는 이미 지웠는데 캐시만 남은 **고아 캐시**도 찾아냅니다.
- **정리** — 앱이나 패키지를 지울 때 남는 설정·캐시까지 함께 정리하고, 이미 지운 앱이 남긴 데이터도 찾아 줍니다.

## 안전 원칙

- 먼저 **무엇을 지울지 계획만** 보여 주고, 확인한 뒤에만 실행합니다.
- 바로 지우지 않고 **휴지통으로** 옮깁니다.
- 시스템 경로와 애플 기본 앱은 건드리지 않습니다. 다른 앱이 함께 쓰는 폴더(Google·Microsoft 등 벤더 공유 폴더)는 남깁니다.
- 확신도를 구분합니다. 확실한 것(번들 ID 일치)과 이름만 비슷한 것을 따로 표시하고, 이름만 비슷한 것은 직접 골라야만 지웁니다.
- 로그인 정보가 든 데이터(Cookies·IndexedDB 등)는 지우지 않습니다. 실행 중인 앱의 캐시와 다시 받기 비싼 모델은 기본으로 제외합니다.

## 설치

```sh
brew install --cask kyungryeol-yoon/tap/macinv   # 예정
```

## 개발

지금은 Go 참고 구현만 있습니다(Go 1.25+).

```sh
git clone https://github.com/kyungryeol-yoon/MacInv.git && cd MacInv
go test ./...
go run . scan        # 전체 스캔 요약 (outdated · trace <이름> · runtimes · disk · clean <이름> · update · orphans)
```

정리 명령(`clean`·`update`·`orphans`·`disk`)은 기본으로 계획만 출력하고, `--apply`를 붙여야 확인 후 실행합니다.

개발 메모와 설계 결정은 [`CLAUDE.md`](./CLAUDE.md)에 있습니다.

## 라이선스

MIT

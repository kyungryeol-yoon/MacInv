#!/bin/zsh
# MacInv — 빌드해서 build/MacInv.app 과 배포용 zip을 만든다(Xcode 프로젝트 없이 swift build만)
#   ./build.sh           빌드 + zip
#   ./build.sh install   빌드 + zip + 「응용 프로그램」 폴더에 복사하고 다시 실행
# 버전: VERSION 환경 변수 > 가장 가까운 git 태그(v1.2.3 → 1.2.3) > 0.0.0
# 빌드 번호: 커밋 수(늘 커짐)
set -euo pipefail
cd "$(dirname "$0")"
VERSION=${VERSION:-$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null | sed 's/^v//' || true)}
VERSION=${VERSION:-0.0.0}
BUILD=$(git rev-list --count HEAD 2>/dev/null || echo 1)

swift build -c release
APP=build/MacInv.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp .build/release/MacInv "$APP/Contents/MacOS/"
cp packaging/AppIcon.icns "$APP/Contents/Resources/"   # 아이콘을 바꾸려면 packaging/make-icon.swift
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>MacInv</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundleIdentifier</key><string>io.github.kyungryeol-yoon.macinv</string>
  <key>CFBundleName</key><string>MacInv</string>
  <key>CFBundleDisplayName</key><string>MacInv</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$BUILD</string>
  <key>LSMinimumSystemVersion</key><string>15.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>© 2026 Yoon · MIT License</string>
</dict>
</plist>
PLIST
# 애플 개발자 서명 없이 임시(ad-hoc) 서명
codesign --force --sign - "$APP"
ZIP=build/MacInv-$VERSION.zip
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"
echo "만듦: $APP ($VERSION, 빌드 $BUILD) · $ZIP"
echo "sha256: $(shasum -a 256 "$ZIP" | cut -d' ' -f1)"

if [[ "${1:-}" == install ]]; then
  pkill -x MacInv || true
  sleep 1
  rm -rf /Applications/MacInv.app
  cp -R "$APP" /Applications/
  open /Applications/MacInv.app || { sleep 2; open /Applications/MacInv.app; }   # 막 끈 직후엔 가끔 실패(-600)
  echo "설치: /Applications/MacInv.app"
fi

// swift-tools-version: 6.0
import PackageDescription

let package = Package(
  name: "MacInv",
  platforms: [.macOS(.v15)],
  targets: [
    // 로직(화면 없음) — 스캔·판정·실행. 테스트 대상
    .target(name: "MacInvCore", path: "Sources/MacInvCore"),
    // 메뉴 막대 앱(SwiftUI)
    .executableTarget(name: "MacInv", dependencies: ["MacInvCore"], path: "Sources/MacInv"),
    .testTarget(name: "MacInvCoreTests", dependencies: ["MacInvCore"], path: "Tests/MacInvCoreTests"),
  ],
  swiftLanguageModes: [.v5]
)

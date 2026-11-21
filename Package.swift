// swift-tools-version: 6.0
import PackageDescription

let package = Package(
  name: "MacInv",
  platforms: [.macOS(.v15)],
  targets: [
    .executableTarget(
      name: "MacInv",
      path: "Sources/MacInv"
    )
  ],
  swiftLanguageModes: [.v5]
)

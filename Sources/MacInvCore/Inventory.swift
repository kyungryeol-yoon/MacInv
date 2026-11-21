import Foundation

/// 이 맥에 설치된 것 전체(읽기 전용 스캔 결과).
public struct Inventory: Sendable {
  public var apps: [AppItem] = []
  public var formulae: [BrewFormula] = []
  public var casks: [BrewCask] = []
  public var packages: [PackageItem] = []
  /// 실패했지만 스캔은 계속한 것(사용자에게 보여 준다)
  public var warnings: [String] = []
  public var scannedAt = Date()
  public init() {}
}

/// 앱이 어디서 왔는지.
public enum AppOrigin: Sendable, Equatable, Hashable {
  case apple
  case appStore
  case brewCask(String)
  case other

  public var label: String {
    switch self {
    case .apple: return "Apple"
    case .appStore: return "App Store"
    case .brewCask(let t): return "brew cask \(t)"
    case .other: return "직접 설치"
    }
  }
}

public struct AppItem: Sendable, Identifiable, Hashable {
  public var id: String { path }
  public let name: String
  public let path: String
  public let bundleID: String?
  public let version: String?
  public let dateAdded: Date?
  public var origin: AppOrigin
  /// .app 번들 용량(KB). 아직 안 쟀으면 nil
  public var bundleKB: Int?

  public init(name: String, path: String, bundleID: String?, version: String?, dateAdded: Date?,
              origin: AppOrigin = .other, bundleKB: Int? = nil) {
    self.name = name; self.path = path; self.bundleID = bundleID; self.version = version
    self.dateAdded = dateAdded; self.origin = origin; self.bundleKB = bundleKB
  }
}

public struct BrewFormula: Sendable, Identifiable, Hashable {
  public var id: String { name }
  public let name: String
  public let version: String?
  /// 직접 설치(brew install로 이름을 적어 설치)인지 — false면 다른 것의 의존성으로 딸려 온 것
  public let onRequest: Bool
  public let dependencies: [String]
}

public struct BrewCask: Sendable, Identifiable, Hashable {
  public var id: String { token }
  public let token: String
  public let version: String?
  /// 이 cask가 설치한 .app 이름들(예: "DiSwitch.app")
  public let appNames: [String]
}

public enum PackageManager: String, Sendable, CaseIterable {
  case npm, pipx, cargo, go, mas
  public var label: String {
    switch self {
    case .npm: return "npm 전역"
    case .pipx: return "pipx"
    case .cargo: return "cargo"
    case .go: return "Go 실행 파일"
    case .mas: return "App Store(mas)"
    }
  }
}

public struct PackageItem: Sendable, Identifiable, Hashable {
  public var id: String { "\(manager.rawValue):\(name)" }
  public let name: String
  public let version: String?
  public let manager: PackageManager
  public let path: String?
}

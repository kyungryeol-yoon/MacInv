import Foundation

/// 이 맥에 설치된 것 전체(읽기 전용 스캔 결과).
public struct Inventory: Sendable {
  public var apps: [AppItem] = []
  public var formulae: [BrewFormula] = []
  public var casks: [BrewCask] = []
  public var packages: [PackageItem] = []
  public var runtimes: [Runtime] = []
  public var versionManagers: [VersionManagerInfo] = []
  public var background: [BackgroundItem] = []
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
  /// 번들 밖 데이터(~/Library 등) 용량(KB) — 측정 전용 규칙(Paths.sizePaths). 데이터가 없으면 0
  public var dataKB: Int?
  /// 권한(전체 디스크 접근)이 없어 데이터 일부를 못 셈 — 실제보다 적다
  public var dataPartial = false
  /// 권한이 없어 못 읽은 폴더(잰 경로 단위)
  public var deniedPaths: [String] = []
  /// 번들 + 데이터. 번들은 앱을 지워야 줄고, 데이터는 정리로 줄일 여지가 있다
  public var totalKB: Int? { bundleKB == nil && dataKB == nil ? nil : (bundleKB ?? 0) + (dataKB ?? 0) }

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
  /// pkg 설치기를 쓴다 — 업그레이드·삭제에 sudo 암호가 필요해서 GUI에서는 입력할 곳이 없어 멈춘다
  public var usesInstaller = false
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

/// Homebrew 의존 관계(순수).
public enum BrewGraph {
  /// 포뮬러 → 그것을 의존성으로 쓰는 설치된 포뮬러들(이름순). 지우면 이것들이 깨진다.
  public static func dependents(_ formulae: [BrewFormula]) -> [String: [String]] {
    var out: [String: [String]] = [:]
    for f in formulae { for d in f.dependencies { out[d, default: []].append(f.name) } }
    return out.mapValues { $0.sorted() }
  }

  /// 의존성으로 깔렸는데 이제 아무것도 안 쓰는 것 — `brew autoremove` 대상.
  public static func unused(_ formulae: [BrewFormula]) -> [String] {
    let deps = dependents(formulae)
    return formulae.filter { !$0.onRequest && (deps[$0.name] ?? []).isEmpty }.map(\.name).sorted()
  }
}

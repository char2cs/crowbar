import SwiftUI

enum TabIcon {
  static func of(kind: String, path: String?) -> CrowbarIcon {
    switch kind {
    case "terminal": .terminalWindow
    case "review": .gitPullRequest
    case "diff": .gitBranch
    case "browser": .globe
    case "preview": .eye
    case "newTab": .plus
    default: file(path)
    }
  }

  private static func file(_ path: String?) -> CrowbarIcon {
    switch path.map({ ($0 as NSString).pathExtension.lowercased() }) {
    case "md", "mdx", "txt": .fileText
    case "png", "jpg", "jpeg", "gif", "svg", "webp": .fileImage
    case "swift", "json", "yaml", "yml", "toml", "ts", "tsx", "js", "jsx", "go", "rs", "py", "rb",
      "c", "h", "cpp", "java", "kt", "css", "html", "sh":
      .fileCode
    default: .file
    }
  }
}

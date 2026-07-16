import type { CoreFeaturesState } from './feature'
import type {
  FooterLeadingItemId,
  FooterTrailingItemId,
  HeaderTrailingItemId,
  SidebarActivityItemId,
} from '@/features/layout/config/item-order'

export type Theme = string
export type ThemeMode = 'light' | 'dark' | 'system'
export type RenderWhitespaceMode = 'none' | 'boundary' | 'trailing' | 'all'
export type EditorEngine = 'monaco' | 'nvim' | 'helix' | 'vim' | 'custom'

export interface Settings {
  // General
  autoSave: boolean
  sidebarPosition: 'left' | 'right'
  quickOpenPreview: boolean
  // Editor
  fontFamily: string
  editorEngine: EditorEngine
  fontSize: number
  editorLineHeight: number
  tabSize: number
  wordWrap: boolean
  lineNumbers: boolean
  renderWhitespace: RenderWhitespaceMode
  renderIndentGuides: boolean
  semanticHighlighting: boolean
  highlightOccurrences: boolean
  showMinimap: boolean
  // Terminal
  terminalFontFamily: string
  terminalFontSize: number
  terminalLineHeight: number
  terminalLetterSpacing: number
  terminalScrollback: number
  terminalCursorStyle: 'block' | 'underline' | 'bar'
  terminalCursorBlink: boolean
  terminalCursorWidth: number
  terminalDefaultShellId: string
  terminalDefaultProfileId: string
  // UI
  uiFontFamily: string
  uiFontSize: number
  // Theme
  theme: Theme
  iconTheme: string
  themeMode: ThemeMode
  syncSystemTheme: boolean // deprecated — kept for migration only
  autoThemeLight: Theme // deprecated — kept for migration only
  autoThemeDark: Theme // deprecated — kept for migration only
  nativeMenuBar: boolean
  compactMenuBar: boolean
  windowTransparency: boolean
  sidebarTabsPosition: 'top' | 'left'
  titleBarProjectMode: 'tabs' | 'window'
  headerTrailingItemsOrder: HeaderTrailingItemId[]
  sidebarActivityItemsOrder: Array<SidebarActivityItemId | string>
  footerLeadingItemsOrder: FooterLeadingItemId[]
  footerTrailingItemsOrder: FooterTrailingItemId[]
  openFoldersInNewWindow: boolean
  // Layout
  sidebarWidth: number
  // Keyboard
  keybindingPreset: 'none' | 'vscode' | 'jetbrains' | 'sublime' | 'xcode' | 'atom' | 'emacs' | 'zed'
  // Language
  defaultLanguage: string
  autoDetectLanguage: boolean
  formatOnSave: boolean
  formatter: string
  lintOnSave: boolean
  autoCompletion: boolean
  parameterHints: boolean
  // External Editor
  externalEditor: 'none' | 'nvim' | 'helix' | 'vim' | 'custom'
  customEditorCommand: string
  // Features
  coreFeatures: CoreFeaturesState
  // Advanced
  enterpriseManagedMode: boolean
  enterpriseRequireExtensionAllowlist: boolean
  enterpriseAllowedExtensionIds: string[]
  showFpsOverlay: boolean
  /**
   * How long (minutes) a workspace stays mounted in memory after you switch
   * away, so switching back is instant. 0 destroys it on switch (the old
   * behaviour). Capped at RETENTION_CAP workspaces regardless of this value.
   */
  workspaceKeepAliveMinutes: number
  // Other
  extensionsActiveTab:
    | 'all'
    | 'core'
    | 'language'
    | 'theme'
    | 'icon-theme'
    | 'snippet'
    | 'database'
    | 'skill'
    | 'agent'
  maxOpenTabs: number
  //// File tree
  fileTreeIndentSize: number
  compactFoldersInFileTree: boolean
  fileTreeDensity: 'compact' | 'default' | 'comfortable'
  showHiddenFilesInFileTree: boolean
  showGitignoredFilesInFileTree: boolean
  hiddenFilePatterns: string[]
  hiddenDirectoryPatterns: string[]
  gitChangesFolderView: boolean
  confirmBeforeDiscard: boolean
  autoRefreshGitStatus: boolean
  showUntrackedFiles: boolean
  showStagedFirst: boolean
  gitDefaultDiffView: 'unified' | 'split'
  openDiffOnClick: boolean
  showGitStatusInFileTree: boolean
  compactGitStatusBadges: boolean
  collapseEmptyGitSections: boolean
  rememberLastGitPanelMode: boolean
  gitLastPanelMode: 'changes' | 'history' | 'worktrees'
  gitSidebarTabOrder: Array<'changes' | 'history' | 'worktrees'>
  enableInlineGitBlame: boolean
  enableGitGutter: boolean
  // Telemetry
  telemetry: boolean
}

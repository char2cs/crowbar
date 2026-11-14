import type { CoreFeaturesState } from './feature'
import type { BuildChannel } from '@/lib/build-info'
import type { SearchEngine } from '@/features/browser/lib/search-engines'

export type Theme = string
export type ThemeMode = 'light' | 'dark' | 'system'
export type TabListScope = 'project' | 'all'
type RenderWhitespaceMode = 'none' | 'boundary' | 'trailing' | 'all'

export interface Settings {
  // General
  autoSave: boolean
  sidebarPosition: 'left' | 'right'
  sidebarFollowsFocus: boolean
  /** Which views the tab strip lists: the active project's, or every one open on this client. */
  tabListScope: TabListScope
  // Editor
  fontFamily: string
  fontSize: number
  /** Base type size for the rich markdown editor. See lib/markdown-font-size.ts. */
  markdownFontSize: number
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
  // Browser
  /** URL a new browser tab opens on; empty opens the blank address bar. */
  browserHomepage: string
  browserSearchEngine: SearchEngine
  /** Search URL for the 'custom' engine; `%s` stands for the query. */
  browserCustomSearchUrl: string
  /** Page zoom in percent, applied when a browser tab's webview is created. */
  browserZoom: number
  // UI
  uiFontFamily: string
  uiFontSize: number
  headingFontFamily: string
  // Agents
  /** Whether a chat opens on Chat rather than Terminal; only the landing surface, both stay
   *  reachable from the chat's switcher. */
  chatIsDefaultPresentation: boolean
  /** Dev-only diagnostic: adds a Split surface (reconstructed chat beside its live TUI). Off by
   *  default; the switch exists only in development builds. */
  chatSplitPresentationEnabled: boolean
  // Theme
  theme: Theme
  iconTheme: string
  themeMode: ThemeMode
  syncSystemTheme: boolean // deprecated — kept for migration only
  autoThemeLight: Theme // deprecated — kept for migration only
  autoThemeDark: Theme // deprecated — kept for migration only
  windowTransparency: boolean
  // Layout
  sidebarWidth: number
  // Keyboard
  // Language
  formatOnSave: boolean
  /** Show the cursor line's git blame after the line. */
  inlineBlame: boolean
  autoCompletion: boolean
  parameterHints: boolean
  // Features
  coreFeatures: CoreFeaturesState
  // Advanced
  buildReadout: boolean
  /** Sidebar-header build indicator. 'auto' detects dev/nightly/beta/release from the build; any other value forces that state for QA, and 'off' hides it. */
  buildBadgeOverride: 'auto' | 'off' | BuildChannel
  // Other
  maxOpenTabs: number
  //// File tree
  fileTreeIndentSize: number
  compactFoldersInFileTree: boolean
  fileTreeDensity: 'compact' | 'default' | 'comfortable'
  showHiddenFilesInFileTree: boolean
  showGitignoredFilesInFileTree: boolean
  hiddenFilePatterns: string[]
  hiddenDirectoryPatterns: string[]
  showGitStatusInFileTree: boolean
  compactGitStatusBadges: boolean
  //// Git
  /** Branch Review's diff layout; a display preference, so global rather than per-workspace. */
  diffViewMode: 'split' | 'unified'
}

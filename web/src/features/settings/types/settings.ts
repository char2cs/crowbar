import type { CoreFeaturesState } from './feature'
import type { BuildChannel } from '@/lib/build-info'

export type Theme = string
export type ThemeMode = 'light' | 'dark' | 'system'
export type RenderWhitespaceMode = 'none' | 'boundary' | 'trailing' | 'all'
export type EditorEngine = 'monaco' | 'nvim' | 'helix' | 'vim' | 'custom'

export interface Settings {
  // General
  autoSave: boolean
  sidebarPosition: 'left' | 'right'
  // Editor
  fontFamily: string
  editorEngine: EditorEngine
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
  // UI
  uiFontFamily: string
  uiFontSize: number
  // Agents
  /**
   * Whether a chat lands on Chat rather than Terminal when it opens. User-level,
   * not per-workspace, and it only picks the LANDING surface — both stay
   * reachable from the chat's own surface switcher whichever way this sits.
   */
  chatIsDefaultPresentation: boolean
  /**
   * DEV-ONLY DIAGNOSTIC. Adds a third surface — Split — to a chat's surface
   * switcher, showing the reconstructed chat and its live TUI side by side so a
   * discrepancy between the two is visible at a glance. Off by default, and the
   * switch that turns it on only exists in a development build.
   */
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
  // External Editor
  externalEditor: 'none' | 'nvim' | 'helix' | 'vim' | 'custom'
  customEditorCommand: string
  // Features
  coreFeatures: CoreFeaturesState
  // Advanced
  showFpsOverlay: boolean
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
  /** Branch Review's diff toolbar toggle (review-diff-tab.tsx) — 'split'
   *  (side-by-side) or 'unified' (inline). A display preference, not
   *  per-workspace data, so it lives here like sidebarPosition/theme rather
   *  than in a workspace store. */
  diffViewMode: 'split' | 'unified'
}

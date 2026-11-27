import type { CoreFeaturesState } from './feature'
import type { BuildChannel } from '@/lib/build-info'
import type { SearchEngine } from '@/features/browser/lib/search-engines'

export type Theme = string
export type ThemeMode = 'light' | 'dark' | 'system'
export type TabListScope = 'project' | 'all'
type RenderWhitespaceMode = 'none' | 'boundary' | 'trailing' | 'all'

export interface Settings {
  autoSave: boolean
  sidebarPosition: 'left' | 'right'
  sidebarFollowsFocus: boolean
  tabListScope: TabListScope
  fontFamily: string
  fontSize: number
  markdownFontSize: number
  editorLineHeight: number
  tabSize: number
  wordWrap: boolean
  lineNumbers: boolean
  renderWhitespace: RenderWhitespaceMode
  renderIndentGuides: boolean
  semanticHighlighting: boolean
  highlightOccurrences: boolean
  terminalFontFamily: string
  terminalFontSize: number
  terminalLineHeight: number
  terminalLetterSpacing: number
  terminalScrollback: number
  terminalCursorStyle: 'block' | 'underline' | 'bar'
  terminalCursorBlink: boolean
  terminalCursorWidth: number
  browserHomepage: string
  browserSearchEngine: SearchEngine
  browserCustomSearchUrl: string
  browserZoom: number
  uiFontFamily: string
  uiFontSize: number
  headingFontFamily: string
  chatIsDefaultPresentation: boolean
  chatSplitPresentationEnabled: boolean
  theme: Theme
  iconTheme: string
  themeMode: ThemeMode
  syncSystemTheme: boolean
  autoThemeLight: Theme
  autoThemeDark: Theme
  windowTransparency: boolean
  sidebarWidth: number
  formatOnSave: boolean
  inlineBlame: boolean
  autoCompletion: boolean
  parameterHints: boolean
  coreFeatures: CoreFeaturesState
  buildReadout: boolean
  buildBadgeOverride: 'auto' | 'off' | BuildChannel
  maxOpenTabs: number
  fileTreeIndentSize: number
  compactFoldersInFileTree: boolean
  fileTreeDensity: 'compact' | 'default' | 'comfortable'
  showHiddenFilesInFileTree: boolean
  showGitignoredFilesInFileTree: boolean
  hiddenFilePatterns: string[]
  hiddenDirectoryPatterns: string[]
  showGitStatusInFileTree: boolean
  compactGitStatusBadges: boolean
  diffViewMode: 'split' | 'unified'
}

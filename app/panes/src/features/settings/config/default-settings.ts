import {
  MARKDOWN_FONT_SIZE_DEFAULT,
  normalizeMarkdownFontSize,
} from '@/features/settings/lib/markdown-font-size'
import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT } from '@/features/settings/lib/ui-font-size'
import {
  DEFAULT_CODE_FONT_SIZE,
  DEFAULT_HEADING_FONT_FAMILY,
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_TERMINAL_FONT_FAMILY,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import type { Settings } from '@/features/settings/types/settings'

export const defaultSettings: Settings = {
  autoSave: false,
  sidebarPosition: 'left',
  sidebarFollowsFocus: false,
  tabListScope: 'project',
  fontFamily: DEFAULT_MONO_FONT_FAMILY,
  fontSize: DEFAULT_CODE_FONT_SIZE,
  markdownFontSize: MARKDOWN_FONT_SIZE_DEFAULT,
  editorLineHeight: 1.4,
  tabSize: 2,
  wordWrap: false,
  lineNumbers: true,
  renderWhitespace: 'none',
  renderIndentGuides: true,
  semanticHighlighting: true,
  highlightOccurrences: false,
  terminalFontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
  terminalFontSize: DEFAULT_CODE_FONT_SIZE,
  terminalLineHeight: 1,
  terminalLetterSpacing: 0,
  terminalScrollback: 10000,
  terminalCursorStyle: 'block',
  terminalCursorBlink: true,
  terminalCursorWidth: 2,
  browserHomepage: '',
  browserSearchEngine: 'duckduckgo',
  browserCustomSearchUrl: '',
  browserZoom: 100,
  uiFontFamily: DEFAULT_UI_FONT_FAMILY,
  uiFontSize: UI_FONT_SIZE_DEFAULT,
  headingFontFamily: DEFAULT_HEADING_FONT_FAMILY,
  chatIsDefaultPresentation: true,
  chatSplitPresentationEnabled: false,
  theme: 'crowbar',
  iconTheme: 'material',
  themeMode: 'system',
  syncSystemTheme: false,
  autoThemeLight: 'crowbar-light',
  autoThemeDark: 'crowbar-dark',
  windowTransparency: true,
  sidebarWidth: 220,
  formatOnSave: false,
  inlineBlame: true,
  autoCompletion: true,
  parameterHints: true,
  coreFeatures: {
    breadcrumbs: true,
  },
  buildReadout: import.meta.env.DEV,
  buildBadgeOverride: 'auto',
  maxOpenTabs: 100,
  fileTreeIndentSize: 16,
  compactFoldersInFileTree: false,
  fileTreeDensity: 'default',
  showHiddenFilesInFileTree: true,
  showGitignoredFilesInFileTree: true,
  hiddenFilePatterns: [],
  hiddenDirectoryPatterns: [],
  showGitStatusInFileTree: true,
  compactGitStatusBadges: false,
  diffViewMode: 'split',
}

export const getDefaultSetting = <K extends keyof Settings>(key: K): Settings[K] =>
  defaultSettings[key]

export function getDefaultSettingsSnapshot(): Settings {
  return {
    ...defaultSettings,
    coreFeatures: { ...defaultSettings.coreFeatures },
    hiddenFilePatterns: [...defaultSettings.hiddenFilePatterns],
    hiddenDirectoryPatterns: [...defaultSettings.hiddenDirectoryPatterns],
    uiFontSize: normalizeUiFontSize(defaultSettings.uiFontSize),
    markdownFontSize: normalizeMarkdownFontSize(defaultSettings.markdownFontSize),
  }
}

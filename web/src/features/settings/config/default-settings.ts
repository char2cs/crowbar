import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT } from '@/features/settings/lib/ui-font-size'
import {
  DEFAULT_CODE_FONT_SIZE,
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_TERMINAL_FONT_FAMILY,
  DEFAULT_TERMINAL_FONT_SIZE,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import type { Settings } from '@/features/settings/types/settings'

export const defaultSettings: Settings = {
  // General
  autoSave: false,
  sidebarPosition: 'left',
  // Editor
  fontFamily: DEFAULT_MONO_FONT_FAMILY,
  editorEngine: 'monaco',
  fontSize: DEFAULT_CODE_FONT_SIZE,
  editorLineHeight: 1.4,
  tabSize: 2,
  wordWrap: false,
  lineNumbers: true,
  renderWhitespace: 'none',
  renderIndentGuides: true,
  semanticHighlighting: true,
  highlightOccurrences: false,
  showMinimap: false,
  // Terminal
  terminalFontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
  terminalFontSize: DEFAULT_TERMINAL_FONT_SIZE,
  terminalLineHeight: 1,
  terminalLetterSpacing: 0,
  terminalScrollback: 10000,
  terminalCursorStyle: 'block',
  terminalCursorBlink: true,
  terminalCursorWidth: 2,
  // UI
  uiFontFamily: DEFAULT_UI_FONT_FAMILY,
  uiFontSize: UI_FONT_SIZE_DEFAULT,
  // Theme
  theme: 'crowbar',
  iconTheme: 'material',
  themeMode: 'system',
  syncSystemTheme: false,
  autoThemeLight: 'crowbar-light',
  autoThemeDark: 'crowbar-dark',
  windowTransparency: true,
  // Layout
  sidebarWidth: 220,
  // Keyboard
  // Language
  formatOnSave: false,
  formatter: 'prettier',
  lintOnSave: false,
  autoCompletion: true,
  parameterHints: true,
  // External Editor
  externalEditor: 'none',
  customEditorCommand: '',
  // Features
  coreFeatures: {
    breadcrumbs: true,
  },
  // Advanced
  showFpsOverlay: false,
  workspaceKeepAliveMinutes: 10,
  // Other
  maxOpenTabs: 100,
  //// File tree
  fileTreeIndentSize: 16,
  compactFoldersInFileTree: false,
  fileTreeDensity: 'default',
  showHiddenFilesInFileTree: true,
  showGitignoredFilesInFileTree: true,
  hiddenFilePatterns: [],
  hiddenDirectoryPatterns: [],
  showGitStatusInFileTree: true,
  compactGitStatusBadges: false,
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
  }
}

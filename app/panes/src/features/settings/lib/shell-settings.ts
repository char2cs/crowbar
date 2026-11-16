import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { normalizeSettings } from '@/features/settings/lib/settings-normalization'
import type { Settings } from '@/features/settings/types/settings'

// The settings the shell owns for panes: the keys of its `settings` frame (PANE_SETTING_KEYS in the protocol crate).
const SHELL_KEYS: ReadonlySet<string> = new Set([
  'autoSave',
  'fontFamily',
  'fontSize',
  'markdownFontSize',
  'editorLineHeight',
  'tabSize',
  'wordWrap',
  'lineNumbers',
  'renderWhitespace',
  'renderIndentGuides',
  'semanticHighlighting',
  'highlightOccurrences',
  'showMinimap',
  'formatOnSave',
  'inlineBlame',
  'autoCompletion',
  'parameterHints',
  'maxOpenTabs',
  'iconTheme',
  'uiFontFamily',
  'uiFontSize',
  'headingFontFamily',
  'terminalFontFamily',
  'terminalFontSize',
  'terminalLineHeight',
  'terminalLetterSpacing',
  'terminalScrollback',
  'terminalCursorStyle',
  'terminalCursorBlink',
  'terminalCursorWidth',
  'browserHomepage',
  'browserSearchEngine',
  'browserCustomSearchUrl',
  'browserZoom',
  'chatIsDefaultPresentation',
  'fileTreeIndentSize',
  'compactFoldersInFileTree',
  'fileTreeDensity',
  'showHiddenFilesInFileTree',
  'showGitignoredFilesInFileTree',
  'hiddenFilePatterns',
  'hiddenDirectoryPatterns',
  'showGitStatusInFileTree',
  'compactGitStatusBadges',
  'diffViewMode',
])

function sameShape(fallback: unknown, value: unknown): boolean {
  if (Array.isArray(fallback)) return Array.isArray(value)
  return typeof value === typeof fallback
}

/** The pane's settings once the shell's `values` apply: a shell-owned key the shell did not send, or sent in the wrong shape, has its default; every other key (the theme, which the theme tokens set, and pane-local ones) stays as it is. */
export function resolveShellSettings(
  current: Settings,
  values: Readonly<Record<string, unknown>>,
): Settings {
  const defaults = getDefaultSettingsSnapshot() as unknown as Record<string, unknown>
  const next = { ...current } as unknown as Record<string, unknown>
  for (const key of SHELL_KEYS) {
    const sent = key in values && sameShape(defaults[key], values[key])
    next[key] = sent ? values[key] : defaults[key]
  }
  return normalizeSettings(next as unknown as Settings)
}

type SettingsWriter = (key: string, value: unknown) => void

let writer: SettingsWriter | null = null

/** Sends writes to shell-owned settings to the shell instead of keeping them in the pane; returns the stop. */
export function routeSettingsToShell(send: SettingsWriter): () => void {
  writer = send
  return () => {
    writer = null
  }
}

/** Hands a write to the shell when the key is shell-owned and a route is set; false means the pane keeps the write itself. */
export function writeSettingToShell(key: string, value: unknown): boolean {
  if (!writer || !SHELL_KEYS.has(key)) return false
  writer(key, value)
  return true
}

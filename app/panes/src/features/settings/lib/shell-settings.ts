import { create } from 'zustand'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { normalizeSettings } from '@/features/settings/lib/settings-normalization'
import type { Settings } from '@/features/settings/types/settings'

const SHELL_KEYS: readonly string[] = [
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
]

function sameShape(fallback: unknown, value: unknown): boolean {
  if (Array.isArray(fallback)) return Array.isArray(value)
  return typeof value === typeof fallback
}

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

const useShellRoute = create<{ send: SettingsWriter | null }>(() => ({ send: null }))

export function routeSettingsToShell(send: SettingsWriter): () => void {
  useShellRoute.setState({ send })
  return () => useShellRoute.setState({ send: null })
}

export function writeSettingToShell(key: string, value: unknown): boolean {
  const { send } = useShellRoute.getState()
  if (!send || !SHELL_KEYS.includes(key)) return false
  send(key, value)
  return true
}

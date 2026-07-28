import { themeRegistry } from '@/extensions/themes/theme-registry'
import { normalizeFileTreeDensity } from '@/features/file-explorer/lib/file-tree-density'
import { getDefaultSetting } from '@/features/settings/config/default-settings'
import {
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import { normalizeConfiguredFontFamily } from '@/features/settings/lib/font-family-resolution'
import { normalizeMarkdownFontSize } from '@/features/settings/lib/markdown-font-size'
import { normalizeUiFontSize } from '@/features/settings/lib/ui-font-size'
import type { Settings } from '@/features/settings/types/settings'

const LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT = 1.2
const TERMINAL_LINE_HEIGHT_DEFAULT = 1
const EDITOR_LINE_HEIGHT_MIN = 1
const EDITOR_LINE_HEIGHT_MAX = 2
const FILE_TREE_INDENT_SIZE_MIN = 8
const FILE_TREE_INDENT_SIZE_MAX = 32
const WORKSPACE_KEEP_ALIVE_MIN = 0
const WORKSPACE_KEEP_ALIVE_MAX = 120
const WORKSPACE_KEEP_ALIVE_DEFAULT = 10
const RENDER_WHITESPACE_MODES = new Set<Settings['renderWhitespace']>([
  'none',
  'boundary',
  'trailing',
  'all',
])
const EDITOR_ENGINES = new Set<Settings['editorEngine']>([
  'monaco',
  'nvim',
  'helix',
  'vim',
  'custom',
])
const EXTERNAL_EDITOR_MODES = new Set<Settings['externalEditor']>([
  'none',
  'nvim',
  'helix',
  'vim',
  'custom',
])

function normalizeEditorLineHeight(value: number): number {
  if (!Number.isFinite(value)) {
    return 1.4
  }

  const snapped = Math.round(value * 10) / 10
  return Math.min(EDITOR_LINE_HEIGHT_MAX, Math.max(EDITOR_LINE_HEIGHT_MIN, snapped))
}

function normalizeFileTreeIndentSize(value: number): number {
  if (!Number.isFinite(value)) {
    return 20
  }

  const snapped = Math.round(value)
  return Math.min(FILE_TREE_INDENT_SIZE_MAX, Math.max(FILE_TREE_INDENT_SIZE_MIN, snapped))
}

function normalizeWorkspaceKeepAliveMinutes(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return WORKSPACE_KEEP_ALIVE_DEFAULT
  }

  const snapped = Math.round(value)
  return Math.min(WORKSPACE_KEEP_ALIVE_MAX, Math.max(WORKSPACE_KEEP_ALIVE_MIN, snapped))
}

/**
 * `Theme` is a bare `string`, so a persisted id survives the theme it names being
 * renamed or removed ('terra' → 'zen'). Coerce an id the registry doesn't know
 * back to the default at READ time — a graceful fallback, not a migration pass —
 * so no consumer has to guess what a dead id means.
 *
 * The registry is the source of truth, so a theme uploaded this session (the only
 * way a non-builtin id exists — uploads aren't persisted) still round-trips.
 */
function normalizeTheme(value: unknown): Settings['theme'] {
  if (typeof value === 'string' && themeRegistry.getTheme(value)) {
    return value
  }

  return getDefaultSetting('theme')
}

function isRenderWhitespaceMode(value: unknown): value is Settings['renderWhitespace'] {
  return (
    typeof value === 'string' && RENDER_WHITESPACE_MODES.has(value as Settings['renderWhitespace'])
  )
}

function normalizeRenderWhitespace(value: unknown): Settings['renderWhitespace'] {
  if (isRenderWhitespaceMode(value)) {
    return value
  }

  return 'none'
}

function normalizeEditorEngine(
  value: unknown,
  _customEditorCommand: string | undefined,
): Settings['editorEngine'] {
  if (!EDITOR_ENGINES.has(value as Settings['editorEngine'])) {
    return 'monaco'
  }

  return value as Settings['editorEngine']
}

function normalizeExternalEditor(
  value: unknown,
  customEditorCommand: string | undefined,
): Settings['externalEditor'] {
  if (!EXTERNAL_EDITOR_MODES.has(value as Settings['externalEditor'])) {
    return 'none'
  }

  if (value === 'custom' && !customEditorCommand?.trim()) {
    return 'none'
  }

  return value as Settings['externalEditor']
}

export function normalizeSettings(settings: Settings): Settings {
  const normalizedSettings = { ...settings }
  normalizedSettings.uiFontSize = normalizeUiFontSize(normalizedSettings.uiFontSize)
  normalizedSettings.markdownFontSize = normalizeMarkdownFontSize(
    (normalizedSettings as { markdownFontSize?: unknown }).markdownFontSize,
  )
  normalizedSettings.fontFamily = normalizeConfiguredFontFamily(
    normalizedSettings.fontFamily,
    DEFAULT_MONO_FONT_FAMILY,
  )
  normalizedSettings.terminalFontFamily = normalizeConfiguredFontFamily(
    normalizedSettings.terminalFontFamily,
    DEFAULT_MONO_FONT_FAMILY,
  )
  normalizedSettings.uiFontFamily = normalizeConfiguredFontFamily(
    normalizedSettings.uiFontFamily,
    DEFAULT_UI_FONT_FAMILY,
  )
  if (normalizedSettings.terminalLineHeight === LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT) {
    normalizedSettings.terminalLineHeight = TERMINAL_LINE_HEIGHT_DEFAULT
  }
  normalizedSettings.editorLineHeight = normalizeEditorLineHeight(
    normalizedSettings.editorLineHeight,
  )
  normalizedSettings.renderWhitespace = normalizeRenderWhitespace(
    (normalizedSettings as { renderWhitespace?: unknown }).renderWhitespace,
  )
  normalizedSettings.externalEditor = normalizeExternalEditor(
    (normalizedSettings as { externalEditor?: unknown }).externalEditor,
    normalizedSettings.customEditorCommand,
  )
  normalizedSettings.editorEngine = normalizeEditorEngine(
    (normalizedSettings as { editorEngine?: unknown }).editorEngine,
    normalizedSettings.customEditorCommand,
  )
  if (
    normalizedSettings.editorEngine === 'custom' &&
    !normalizedSettings.customEditorCommand.trim()
  ) {
    normalizedSettings.editorEngine = 'monaco'
  }
  if (normalizedSettings.externalEditor !== 'none') {
    normalizedSettings.editorEngine = normalizedSettings.externalEditor
  }
  normalizedSettings.fileTreeIndentSize = normalizeFileTreeIndentSize(
    normalizedSettings.fileTreeIndentSize,
  )
  normalizedSettings.workspaceKeepAliveMinutes = normalizeWorkspaceKeepAliveMinutes(
    (normalizedSettings as { workspaceKeepAliveMinutes?: unknown }).workspaceKeepAliveMinutes,
  )
  normalizedSettings.fileTreeDensity = normalizeFileTreeDensity(normalizedSettings.fileTreeDensity)
  normalizedSettings.theme = normalizeTheme((normalizedSettings as { theme?: unknown }).theme)

  if (!normalizedSettings.themeMode) {
    normalizedSettings.themeMode = normalizedSettings.syncSystemTheme ? 'system' : 'light'
  }

  return normalizedSettings
}

export function normalizeSettingValue<K extends keyof Settings>(
  key: K,
  value: Settings[K],
): Settings[K] {
  if (key === 'uiFontSize') {
    return normalizeUiFontSize(value as number) as Settings[K]
  }

  if (key === 'markdownFontSize') {
    return normalizeMarkdownFontSize(value) as Settings[K]
  }

  if (key === 'fontFamily') {
    return normalizeConfiguredFontFamily(value as string, DEFAULT_MONO_FONT_FAMILY) as Settings[K]
  }

  if (key === 'terminalFontFamily') {
    return normalizeConfiguredFontFamily(value as string, DEFAULT_MONO_FONT_FAMILY) as Settings[K]
  }

  if (key === 'uiFontFamily') {
    return normalizeConfiguredFontFamily(value as string, DEFAULT_UI_FONT_FAMILY) as Settings[K]
  }

  if (key === 'terminalLineHeight' && value === LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT) {
    return TERMINAL_LINE_HEIGHT_DEFAULT as Settings[K]
  }

  if (key === 'editorLineHeight') {
    return normalizeEditorLineHeight(value as number) as Settings[K]
  }

  if (key === 'renderWhitespace') {
    return normalizeRenderWhitespace(value) as Settings[K]
  }

  if (key === 'editorEngine') {
    return normalizeEditorEngine(value, undefined) as Settings[K]
  }

  if (key === 'fileTreeIndentSize') {
    return normalizeFileTreeIndentSize(value as number) as Settings[K]
  }

  if (key === 'workspaceKeepAliveMinutes') {
    return normalizeWorkspaceKeepAliveMinutes(value) as Settings[K]
  }

  if (key === 'fileTreeDensity') {
    return normalizeFileTreeDensity(value as string) as Settings[K]
  }

  if (key === 'theme') {
    return normalizeTheme(value) as Settings[K]
  }

  if (key === 'themeMode') {
    const valid: string[] = ['light', 'dark', 'system']
    return (valid.includes(value as string) ? value : 'system') as Settings[K]
  }

  return value
}

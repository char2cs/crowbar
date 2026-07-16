import { isKeybindingPreset } from '@/features/keymaps/defaults/keybinding-presets'
import { normalizeFileTreeDensity } from '@/features/file-explorer/lib/file-tree-density'
import {
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import { normalizeConfiguredFontFamily } from '@/features/settings/lib/font-family-resolution'
import {
  FOOTER_LEADING_ITEM_IDS,
  FOOTER_TRAILING_ITEM_IDS,
  HEADER_TRAILING_ITEM_IDS,
  SIDEBAR_ACTIVITY_ITEM_IDS,
  normalizeItemOrder,
} from '@/features/layout/config/item-order'
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
  const persistedGitPanelMode = (normalizedSettings as { gitLastPanelMode?: string })
    .gitLastPanelMode

  if (
    persistedGitPanelMode === 'none' ||
    (persistedGitPanelMode && !['changes', 'history', 'worktrees'].includes(persistedGitPanelMode))
  ) {
    normalizedSettings.gitLastPanelMode = 'changes'
  }

  normalizedSettings.uiFontSize = normalizeUiFontSize(normalizedSettings.uiFontSize)
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

  if (!isKeybindingPreset(normalizedSettings.keybindingPreset)) {
    normalizedSettings.keybindingPreset = 'none'
  }

  if (!normalizedSettings.themeMode) {
    normalizedSettings.themeMode = normalizedSettings.syncSystemTheme ? 'system' : 'light'
  }

  normalizedSettings.headerTrailingItemsOrder = normalizeItemOrder(
    normalizedSettings.headerTrailingItemsOrder,
    HEADER_TRAILING_ITEM_IDS,
  )
  normalizedSettings.sidebarActivityItemsOrder = normalizeItemOrder(
    normalizedSettings.sidebarActivityItemsOrder,
    SIDEBAR_ACTIVITY_ITEM_IDS,
  )
  normalizedSettings.footerLeadingItemsOrder = normalizeItemOrder(
    normalizedSettings.footerLeadingItemsOrder,
    FOOTER_LEADING_ITEM_IDS,
  )
  normalizedSettings.footerTrailingItemsOrder = normalizeItemOrder(
    normalizedSettings.footerTrailingItemsOrder,
    FOOTER_TRAILING_ITEM_IDS,
  )

  return normalizedSettings
}

export function normalizeSettingValue<K extends keyof Settings>(
  key: K,
  value: Settings[K],
): Settings[K] {
  if (key === 'uiFontSize') {
    return normalizeUiFontSize(value as number) as Settings[K]
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

  if (key === 'keybindingPreset' && !isKeybindingPreset(value as string)) {
    return 'none' as Settings[K]
  }

  if (key === 'themeMode') {
    const valid: string[] = ['light', 'dark', 'system']
    return (valid.includes(value as string) ? value : 'system') as Settings[K]
  }

  return value
}

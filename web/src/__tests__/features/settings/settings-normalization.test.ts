import { describe, expect, it } from 'vitest'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import {
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import {
  normalizeSettings,
  normalizeSettingValue,
} from '@/features/settings/lib/settings-normalization'
import type { ThemeMode } from '@/features/settings/types/settings'

describe('settings normalization', () => {
  it('preserves configured font settings that may exist on the system', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      fontFamily: '"Geist Mono"',
      terminalFontFamily: 'Geist Mono, monospace',
      uiFontFamily: 'Geist',
    })

    expect(normalized.fontFamily).toBe('"Geist Mono"')
    expect(normalized.terminalFontFamily).toBe('Geist Mono, monospace')
    expect(normalized.uiFontFamily).toBe('Geist')
  })

  it('preserves font updates before persisting', () => {
    expect(normalizeSettingValue('fontFamily', 'Geist Mono')).toBe('Geist Mono')
    expect(normalizeSettingValue('terminalFontFamily', 'Geist Mono')).toBe('Geist Mono')
    expect(normalizeSettingValue('uiFontFamily', 'Geist Sans')).toBe('Geist Sans')
  })

  it('falls back for empty font updates', () => {
    expect(normalizeSettingValue('fontFamily', '   ')).toBe(DEFAULT_MONO_FONT_FAMILY)
    expect(normalizeSettingValue('uiFontFamily', '   ')).toBe(DEFAULT_UI_FONT_FAMILY)
  })

  it('clamps and rounds workspaceKeepAliveMinutes and round-trips valid values', () => {
    // Valid values survive a normalizeSettings round-trip untouched.
    expect(
      normalizeSettings({ ...getDefaultSettingsSnapshot(), workspaceKeepAliveMinutes: 0 })
        .workspaceKeepAliveMinutes,
    ).toBe(0)
    expect(
      normalizeSettings({ ...getDefaultSettingsSnapshot(), workspaceKeepAliveMinutes: 45 })
        .workspaceKeepAliveMinutes,
    ).toBe(45)

    // Out-of-range and non-integer values are clamped/rounded on write.
    expect(normalizeSettingValue('workspaceKeepAliveMinutes', -5)).toBe(0)
    expect(normalizeSettingValue('workspaceKeepAliveMinutes', 9999)).toBe(120)
    expect(normalizeSettingValue('workspaceKeepAliveMinutes', 10.6)).toBe(11)
    // A corrupt persisted value falls back to the default.
    expect(
      normalizeSettings({
        ...getDefaultSettingsSnapshot(),
        workspaceKeepAliveMinutes: Number.NaN,
      }).workspaceKeepAliveMinutes,
    ).toBe(10)
  })

  it('migrates the old terminal line-height default to preserve TUI block graphics', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      terminalLineHeight: 1.2,
    })

    expect(normalized.terminalLineHeight).toBe(1)
    expect(normalizeSettingValue('terminalLineHeight', 1.2)).toBe(1)
  })

  it('clamps editor line height to the supported range', () => {
    expect(normalizeSettingValue('editorLineHeight', 0.6)).toBe(1)
    expect(normalizeSettingValue('editorLineHeight', 2.6)).toBe(2)
    expect(normalizeSettingValue('editorLineHeight', 1.34)).toBe(1.3)
  })

  it('clamps file tree indent size to the supported range', () => {
    expect(normalizeSettingValue('fileTreeIndentSize', 2)).toBe(8)
    expect(normalizeSettingValue('fileTreeIndentSize', 40)).toBe(32)
    expect(normalizeSettingValue('fileTreeIndentSize', 13.6)).toBe(14)
  })

  it('normalizes unsupported file tree density values', () => {
    expect(normalizeSettingValue('fileTreeDensity', 'compact')).toBe('compact')
    expect(normalizeSettingValue('fileTreeDensity', 'dense' as 'default')).toBe('default')
  })

  it('disables blank custom editor engine settings', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      editorEngine: 'custom',
      customEditorCommand: '',
    })

    expect(normalized.editorEngine).toBe('monaco')
  })

  it('normalizes unsupported editor engines', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      editorEngine: 'emacs' as never,
    })

    expect(normalized.editorEngine).toBe('monaco')
  })

  it('migrates legacy external editor settings into editor engine', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      editorEngine: 'monaco',
      externalEditor: 'helix',
    })

    expect(normalized.editorEngine).toBe('helix')
  })

  describe('themeMode migration', () => {
    it('sets themeMode to system when syncSystemTheme was true', () => {
      const result = normalizeSettings({
        ...getDefaultSettingsSnapshot(),
        themeMode: undefined as unknown as ThemeMode,
        syncSystemTheme: true,
      })
      expect(result.themeMode).toBe('system')
    })

    it('sets themeMode to light when syncSystemTheme was false', () => {
      const result = normalizeSettings({
        ...getDefaultSettingsSnapshot(),
        themeMode: undefined as unknown as ThemeMode,
        syncSystemTheme: false,
      })
      expect(result.themeMode).toBe('light')
    })

    it('preserves existing themeMode when present', () => {
      const result = normalizeSettings({
        ...getDefaultSettingsSnapshot(),
        themeMode: 'dark',
      })
      expect(result.themeMode).toBe('dark')
    })

    it('rejects invalid themeMode and falls back to system', () => {
      expect(normalizeSettingValue('themeMode', 'invalid' as ThemeMode)).toBe('system')
    })
  })
})

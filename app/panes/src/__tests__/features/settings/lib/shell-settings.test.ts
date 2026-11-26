import { describe, expect, it } from 'vitest'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { resolveShellSettings } from '@/features/settings/lib/shell-settings'

const current = { ...getDefaultSettingsSnapshot(), theme: 'frosted', themeMode: 'light' as const }

describe('resolveShellSettings', () => {
  it.each([
    ['a boolean', { wordWrap: true }, 'wordWrap', true],
    ['a number', { tabSize: 8 }, 'tabSize', 8],
    ['a string', { iconTheme: 'vscode' }, 'iconTheme', 'vscode'],
    ['a list', { hiddenFilePatterns: ['*.log'] }, 'hiddenFilePatterns', ['*.log']],
    ['a value the pane normalises', { editorLineHeight: 9 }, 'editorLineHeight', 2],
    ['a value of the wrong type', { wordWrap: 'yes' }, 'wordWrap', false],
    ['a list of the wrong type', { hiddenFilePatterns: 'x' }, 'hiddenFilePatterns', []],
    ['a key the pane does not know', { nope: 1 }, 'tabSize', 2],
  ])('takes %s', (_name, values, key, expected) => {
    const next = resolveShellSettings(current, values) as unknown as Record<string, unknown>
    expect(next[key]).toEqual(expected)
  })

  it('gives a key the shell stopped sending its default', () => {
    const edited = { ...current, wordWrap: true, maxOpenTabs: 3 }
    const next = resolveShellSettings(edited, {})
    expect([next.wordWrap, next.maxOpenTabs]).toEqual([false, 100])
  })

  it('leaves pane-local settings as they are', () => {
    const edited = { ...current, coreFeatures: { breadcrumbs: false } }
    expect(resolveShellSettings(edited, {}).coreFeatures).toEqual({ breadcrumbs: false })
  })

  it('leaves the theme to the theme tokens', () => {
    const next = resolveShellSettings(current, { theme: 'crowbar', themeMode: 'dark' })
    expect([next.theme, next.themeMode]).toEqual(['frosted', 'light'])
  })
})

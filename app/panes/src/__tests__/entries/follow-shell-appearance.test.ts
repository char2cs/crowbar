import { beforeEach, describe, expect, it, vi } from 'vitest'
import { followShellAppearance } from '@/entries/follow-shell-appearance'
import type { ThemeTokens } from '@/sdk/theme-tokens'

const updateSetting = vi.fn()

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: { getState: () => ({ updateSetting }) },
}))

function shellThatSends(tokens: ThemeTokens) {
  return { onTheme: (listener: (tokens: ThemeTokens) => void) => (listener(tokens), () => {}) }
}

describe('followShellAppearance', () => {
  beforeEach(() => updateSetting.mockClear())

  it.each([
    [
      'theme and mode',
      { 'theme-id': 'crowbar', 'theme-type': 'light' },
      [
        ['themeMode', 'light'],
        ['theme', 'crowbar'],
      ],
    ],
    ['only the theme', { 'theme-id': 'nord' }, [['theme', 'nord']]],
    ['an unknown mode is ignored', { 'theme-type': 'sepia' }, []],
    ['other tokens are ignored', { 'color-bg': '#fff' }, []],
  ])('%s', (_name, tokens, calls) => {
    followShellAppearance(shellThatSends(tokens))
    expect(updateSetting.mock.calls).toEqual(calls)
  })
})

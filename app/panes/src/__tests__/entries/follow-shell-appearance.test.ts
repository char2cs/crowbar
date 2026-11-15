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
        ['theme', 'crowbar'],
        ['themeMode', 'light'],
      ],
    ],
    ['only the theme', { 'theme-id': 'nord' }, [['theme', 'nord']]],
    ['an unknown mode is ignored', { 'theme-type': 'sepia' }, []],
    ['other tokens are ignored', { 'color-bg': '#fff' }, []],
  ])('%s', async (_name, tokens, calls) => {
    const ready = Promise.resolve()
    followShellAppearance(shellThatSends(tokens), ready)
    await ready
    await Promise.resolve()
    expect(updateSetting.mock.calls).toEqual(calls)
  })

  it('waits for the settings to load before applying', async () => {
    let load: () => void = () => {}
    const ready = new Promise<void>((resolve) => (load = resolve))
    followShellAppearance(shellThatSends({ 'theme-id': 'nord' }), ready)
    expect(updateSetting).not.toHaveBeenCalled()
    load()
    await ready
    await Promise.resolve()
    expect(updateSetting).toHaveBeenCalledWith('theme', 'nord')
  })
})

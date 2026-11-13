// Pane suites pin macOS (the chord tests dispatch Cmd keydowns and header height is
// per platform, both through IS_MAC) with:
//   vi.mock('@/utils/platform', async () => (await import('@/__tests__/__fixtures__/pane-platform')).platformModule)
export const platform = { mobile: false }

export const platformModule = {
  IS_MAC: true,
  get IS_MOBILE() {
    return platform.mobile
  },
  APPLE_KEYBOARD: true,
}

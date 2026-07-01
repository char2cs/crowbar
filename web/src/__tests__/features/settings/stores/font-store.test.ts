import { beforeEach, describe, expect, it } from 'vitest'
import { useFontStore } from '@/features/settings/stores/font-store'

beforeEach(() => {
  localStorage.clear()
})

describe('font store — bundled fonts', () => {
  it('populates monospace fonts with the bundled cuts (incl. static JetBrains Mono)', async () => {
    await useFontStore.getState().actions.loadMonospaceFonts(true)

    const families = useFontStore.getState().monospaceFonts.map((f) => f.family)
    expect(families).toContain('JetBrains Mono')
    expect(families).toContain('JetBrains Mono Variable')
    // The sans bundled font is not monospace and must not appear here.
    expect(families).not.toContain('CalSansUI')
  })

  it('populates the full available-fonts list (never empty)', async () => {
    await useFontStore.getState().actions.loadAvailableFonts(true)

    const families = useFontStore.getState().availableFonts.map((f) => f.family)
    expect(families.length).toBeGreaterThan(0)
    expect(families).toContain('CalSansUI')
    expect(families).toContain('JetBrains Mono')
  })
})

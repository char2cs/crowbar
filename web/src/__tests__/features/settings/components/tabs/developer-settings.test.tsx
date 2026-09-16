import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'

let buildBadgeOverride = 'auto'
const updateSetting = vi.fn()
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) =>
    sel({ settings: { buildBadgeOverride }, search: { query: '' }, updateSetting }),
  getDefaultSetting: (key: string) => (key === 'buildBadgeOverride' ? 'auto' : undefined),
}))

import { BuildBadgeSection } from '@/features/settings/components/tabs/developer-settings'

beforeEach(() => {
  updateSetting.mockClear()
})

describe('BuildBadgeSection cycle button', () => {
  it('advances auto to the next mode (dev)', async () => {
    buildBadgeOverride = 'auto'
    render(<BuildBadgeSection />)
    await userEvent.click(screen.getByRole('button', { name: /cycle build badge mode/i }))
    expect(updateSetting).toHaveBeenCalledWith('buildBadgeOverride', 'dev')
  })

  it('advances release to off', async () => {
    buildBadgeOverride = 'release'
    render(<BuildBadgeSection />)
    await userEvent.click(screen.getByRole('button', { name: /cycle build badge mode/i }))
    expect(updateSetting).toHaveBeenCalledWith('buildBadgeOverride', 'off')
  })

  it('wraps from the last mode (off) back to the first (auto)', async () => {
    buildBadgeOverride = 'off'
    render(<BuildBadgeSection />)
    await userEvent.click(screen.getByRole('button', { name: /cycle build badge mode/i }))
    expect(updateSetting).toHaveBeenCalledWith('buildBadgeOverride', 'auto')
  })
})

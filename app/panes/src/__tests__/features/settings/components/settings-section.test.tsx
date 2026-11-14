import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

import NumberInput from '@/components/ui/number-input'
import Section, { SettingRow } from '@/features/settings/components/settings-section'
import { useSettingsStore } from '@/features/settings/store'

// Mirrors the hand-written JSX structure of the Appearance tab: multiple
// Sections, each containing SettingRows with label + description.
const AppearanceLikePanel = () => (
  <div>
    <Section title="Theme">
      <SettingRow label="Color Theme" description="Choose your active color theme">
        <button type="button">theme-control</button>
      </SettingRow>
    </Section>
    <Section title="Icons">
      <SettingRow label="Icon Theme" description="Icons displayed in the file tree and tabs">
        <button type="button">icon-control</button>
      </SettingRow>
    </Section>
    <Section title="Typography">
      <SettingRow label="UI Font Family" description="Font family for UI elements">
        <button type="button">font-family-control</button>
      </SettingRow>
      <SettingRow label="UI Font Size" description="Adjust UI text and icon scale">
        <button type="button">font-size-control</button>
      </SettingRow>
    </Section>
    <Section title="Layout">
      <SettingRow label="Sidebar Position" description="Which side the file sidebar appears on">
        <button type="button">sidebar-control</button>
      </SettingRow>
    </Section>
  </div>
)

const setQuery = (query: string) => {
  act(() => {
    useSettingsStore.getState().setSearchQuery(query)
  })
}

afterEach(() => {
  platform.mobile = false
  act(() => {
    useSettingsStore.getState().clearSearch()
  })
})

describe('settings search row filtering', () => {
  it('renders all rows and section headings when the query is empty', () => {
    render(<AppearanceLikePanel />)

    expect(screen.getByText('Color Theme')).toBeInTheDocument()
    expect(screen.getByText('Icon Theme')).toBeInTheDocument()
    expect(screen.getByText('UI Font Family')).toBeInTheDocument()
    expect(screen.getByText('UI Font Size')).toBeInTheDocument()
    expect(screen.getByText('Sidebar Position')).toBeInTheDocument()
    expect(screen.getByText('Typography')).toBeVisible()
  })

  it('searching "font" shows only font rows and hides emptied group headings', () => {
    render(<AppearanceLikePanel />)

    setQuery('font')

    // Matching rows (label match, case-insensitive) stay visible.
    expect(screen.getByText('UI Font Family')).toBeInTheDocument()
    expect(screen.getByText('UI Font Size')).toBeInTheDocument()
    expect(screen.getByText('Typography')).toBeVisible()

    // Non-matching rows no longer render.
    expect(screen.queryByText('Color Theme')).not.toBeInTheDocument()
    expect(screen.queryByText('Icon Theme')).not.toBeInTheDocument()
    expect(screen.queryByText('Sidebar Position')).not.toBeInTheDocument()

    // Sections whose every row was filtered out hide their headings too.
    expect(screen.getByText('Theme')).not.toBeVisible()
    expect(screen.getByText('Icons')).not.toBeVisible()
    expect(screen.getByText('Layout')).not.toBeVisible()
  })

  it('matches against the description, case-insensitively', () => {
    render(<AppearanceLikePanel />)

    setQuery('SIDEBAR APPEARS')

    expect(screen.getByText('Sidebar Position')).toBeInTheDocument()
    expect(screen.getByText('Layout')).toBeVisible()
    expect(screen.queryByText('UI Font Family')).not.toBeInTheDocument()
    expect(screen.getByText('Typography')).not.toBeVisible()
  })

  it('clearing the query restores all rows and headings', () => {
    render(<AppearanceLikePanel />)

    setQuery('font')
    expect(screen.queryByText('Color Theme')).not.toBeInTheDocument()

    setQuery('')

    expect(screen.getByText('Color Theme')).toBeInTheDocument()
    expect(screen.getByText('Icon Theme')).toBeInTheDocument()
    expect(screen.getByText('UI Font Family')).toBeInTheDocument()
    expect(screen.getByText('Sidebar Position')).toBeInTheDocument()
    expect(screen.getByText('Theme')).toBeVisible()
    expect(screen.getByText('Icons')).toBeVisible()
    expect(screen.getByText('Layout')).toBeVisible()
  })

  it('fills the search results once the lazily loaded index lands', async () => {
    setQuery('font')

    await waitFor(() =>
      expect(useSettingsStore.getState().search.results.length).toBeGreaterThan(0),
    )
    expect(useSettingsStore.getState().search.isSearching).toBe(false)
  })
})

// The phone's native Settings hosts these tabs: one line per setting, in a card.
describe('mobile setting rows', () => {
  const rowOf = (label: string) => screen.getByText(label).closest('[data-setting-layout]')

  it('puts the label, its description and the control in one row, inside a card', () => {
    platform.mobile = true
    render(
      <Section title="Cursor">
        <SettingRow label="Blink" description="Whether the cursor blinks">
          <button type="button">blink-control</button>
        </SettingRow>
      </Section>,
    )
    const row = rowOf('Blink')
    expect(row).toHaveAttribute('data-setting-layout', 'inline')
    expect(row).toContainElement(screen.getByText('Whether the cursor blinks'))
    expect(row).toContainElement(screen.getByText('blink-control'))
    expect(row?.closest('[data-settings-card]')).not.toBeNull()
  })

  // The phone sizes every stepper alike so their columns line up; the row finds it by this slot.
  it('wraps a stepper in the number-input slot its row sizes', () => {
    platform.mobile = true
    render(
      <Section title="Terminal">
        <SettingRow label="Scrollback">
          <NumberInput value={10000} onChange={vi.fn()} />
        </SettingRow>
      </Section>,
    )
    const control = rowOf('Scrollback')?.querySelector('[data-slot="number-input"]')
    expect(control).not.toBeNull()
    expect(control?.querySelector('input')).toHaveValue('10000')
  })

  it('stacks a control that cannot sit beside its label, with the same row chrome', () => {
    platform.mobile = true
    render(
      <Section title="Typography">
        <SettingRow label="Font" layout="stacked">
          <button type="button">font-control</button>
        </SettingRow>
      </Section>,
    )
    expect(rowOf('Font')).toHaveAttribute('data-setting-layout', 'stacked')
    expect(rowOf('Font')?.closest('[data-settings-card]')).not.toBeNull()
  })

  it('leaves the desktop rows and sections as they were', () => {
    render(
      <Section title="Cursor">
        <SettingRow label="Blink" layout="stacked">
          <button type="button">blink-control</button>
        </SettingRow>
      </Section>,
    )
    expect(document.querySelector('[data-setting-layout]')).toBeNull()
    expect(document.querySelector('[data-settings-card]')).toBeNull()
  })
})

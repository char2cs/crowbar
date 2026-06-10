import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
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
})

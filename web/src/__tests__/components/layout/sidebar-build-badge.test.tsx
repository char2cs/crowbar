import { render } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'

let buildBadgeOverride: string = 'auto'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) => sel({ settings: { buildBadgeOverride } }),
}))

import {
  SidebarBuildBadgeBand,
  SidebarBuildBadgeLabel,
} from '@/components/layout/sidebar-build-badge'

beforeEach(() => {
  buildBadgeOverride = 'auto'
  document.documentElement.classList.remove('dark')
})

describe('SidebarBuildBadgeLabel', () => {
  it('renders nothing when the override is off', () => {
    buildBadgeOverride = 'off'
    const { container } = render(<SidebarBuildBadgeLabel />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the dev title with a timestamp subtitle when forced to dev', () => {
    buildBadgeOverride = 'dev'
    const { getByText } = render(<SidebarBuildBadgeLabel />)
    expect(getByText('dev')).toBeTruthy()
  })

  it('shows the nightly title with a timestamp, never a version', () => {
    buildBadgeOverride = 'nightly'
    document.documentElement.classList.add('dark')
    const { getByText, queryByText } = render(<SidebarBuildBadgeLabel />)
    expect(getByText('nightly')).toBeTruthy()
    expect(queryByText(/beta/)).toBeNull()
  })

  it('puns nightly as "daily" in light mode, without touching its color', () => {
    buildBadgeOverride = 'nightly'
    const { getByText, queryByText } = render(<SidebarBuildBadgeLabel />)
    expect(getByText('daily')).toBeTruthy()
    expect(queryByText('nightly')).toBeNull()
  })

  it('shows the beta title with its preview version', () => {
    buildBadgeOverride = 'beta'
    const { getByText } = render(<SidebarBuildBadgeLabel />)
    expect(getByText('beta')).toBeTruthy()
    expect(getByText('0.0.0-beta.1')).toBeTruthy()
  })

  it('shows only the version for release, with no channel title', () => {
    buildBadgeOverride = 'release'
    const { queryByText, getByText } = render(<SidebarBuildBadgeLabel />)
    expect(queryByText('release')).toBeNull()
    expect(getByText('0.0.0')).toBeTruthy()
  })

  it('left-aligns by default and right-aligns when align="end"', () => {
    buildBadgeOverride = 'beta'
    const { getByText, rerender } = render(<SidebarBuildBadgeLabel />)
    expect(getByText('beta').parentElement).toHaveClass('items-start')
    expect(getByText('beta').parentElement).not.toHaveClass('items-end')

    rerender(<SidebarBuildBadgeLabel align="end" />)
    expect(getByText('beta').parentElement).toHaveClass('items-end')
    expect(getByText('beta').parentElement).not.toHaveClass('items-start')
  })
})

describe('SidebarBuildBadgeBand', () => {
  it('renders nothing for release (no band)', () => {
    buildBadgeOverride = 'release'
    const { container } = render(<SidebarBuildBadgeBand />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when off', () => {
    buildBadgeOverride = 'off'
    const { container } = render(<SidebarBuildBadgeBand />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a band for dev, nightly, and beta', () => {
    for (const channel of ['dev', 'nightly', 'beta']) {
      buildBadgeOverride = channel
      const { container } = render(<SidebarBuildBadgeBand />)
      expect(container).not.toBeEmptyDOMElement()
    }
  })
})

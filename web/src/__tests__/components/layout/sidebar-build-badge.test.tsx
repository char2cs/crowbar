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

  // The decorative art is authored right-biased and the fade authored
  // fading out to the right — both correct only when the badge text (the
  // true window-border edge) is on this bar's own right, i.e. align="end".
  // On the left (sidebar on the left), both must mirror, or the art lands
  // behind the back/forward/panel-toggle cluster instead of the window edge.
  it('mirrors the decorative art for align="start", leaves it unmirrored for align="end" (default)', () => {
    buildBadgeOverride = 'nightly'
    document.documentElement.classList.add('dark')
    const { container, rerender } = render(<SidebarBuildBadgeBand />)
    const artEnd = container.querySelector('svg')?.parentElement as HTMLElement
    expect(artEnd.style.transform).toBe('')

    rerender(<SidebarBuildBadgeBand align="start" />)
    const artStart = container.querySelector('svg')?.parentElement as HTMLElement
    expect(artStart.style.transform).toBe('scaleX(-1)')
  })

  it('stays opaque at the text/window-edge side (align) and fades toward the opposite (button) side', () => {
    buildBadgeOverride = 'nightly'
    // align="end": text/window-edge on the right — opaque (the gradient's
    // LAST color) must anchor there, i.e. `to right`, not `to left`.
    const { container, rerender } = render(<SidebarBuildBadgeBand align="end" />)
    const fillEnd = container.querySelector('[style*="mask-image"]') as HTMLElement
    expect(fillEnd.style.maskImage).toContain('to right')

    rerender(<SidebarBuildBadgeBand align="start" />)
    const fillStart = container.querySelector('[style*="mask-image"]') as HTMLElement
    expect(fillStart.style.maskImage).toContain('to left')
  })
})

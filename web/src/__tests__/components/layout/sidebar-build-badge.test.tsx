import { act, fireEvent, render } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'

let buildBadgeOverride: string = 'auto'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) => sel({ settings: { buildBadgeOverride } }),
}))

import {
  SidebarBuildBadgeBand,
  SidebarBuildBadgeLabel,
} from '@/components/layout/sidebar-build-badge'
import { useConsoleStore } from '@/features/console/stores/console-store'

beforeEach(() => {
  buildBadgeOverride = 'auto'
  useConsoleStore.setState(useConsoleStore.getInitialState())
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

describe('SidebarBuildBadgeLabel console toggle', () => {
  it('is a button that opens the console on click and closes it on the next', () => {
    buildBadgeOverride = 'beta'
    const { getByRole } = render(<SidebarBuildBadgeLabel />)
    const button = getByRole('button', { name: 'Toggle console' })
    expect(button.getAttribute('aria-expanded')).toBe('false')

    fireEvent.click(button)
    expect(useConsoleStore.getState().open).toBe(true)
    expect(button.getAttribute('aria-expanded')).toBe('true')

    fireEvent.click(button)
    expect(useConsoleStore.getState().open).toBe(false)
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

  // The photo is authored with the badge text on its right; on a left-hand
  // sidebar it must mirror so it hugs the window edge, not the button cluster.
  it('mirrors the sky photo for align="start", leaves it unmirrored for align="end" (default)', () => {
    buildBadgeOverride = 'nightly'
    const { container, rerender } = render(<SidebarBuildBadgeBand />)
    const artEnd = container.querySelector('img')?.parentElement as HTMLElement
    expect(artEnd.style.transform).toBe('')

    rerender(<SidebarBuildBadgeBand align="start" />)
    const artStart = container.querySelector('img')?.parentElement as HTMLElement
    expect(artStart.style.transform).toBe('scaleX(-1)')
  })

  it('shows the halftone sky in light and the star field in dark, from the real theme class', async () => {
    buildBadgeOverride = 'nightly'
    const { container } = render(<SidebarBuildBadgeBand />)
    expect(container.querySelector('img')?.getAttribute('data-band-photo')).toBe('light')

    await act(async () => {
      document.documentElement.classList.add('dark')
    })
    expect(container.querySelector('img')?.getAttribute('data-band-photo')).toBe('dark')
  })

  it('lazy-decodes the photo so it never blocks boot', () => {
    buildBadgeOverride = 'nightly'
    const { container } = render(<SidebarBuildBadgeBand />)
    const img = container.querySelector('img') as HTMLImageElement
    expect(img.getAttribute('loading')).toBe('lazy')
    expect(img.getAttribute('decoding')).toBe('async')
  })

  it('draws no photo for the dev and beta channels', () => {
    for (const channel of ['dev', 'beta']) {
      buildBadgeOverride = channel
      const { container } = render(<SidebarBuildBadgeBand />)
      expect(container.querySelector('img')).toBeNull()
    }
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

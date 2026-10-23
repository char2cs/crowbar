import { render } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'

let buildBadgeOverride: string = 'auto'
let showBuildReadout = true
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) =>
    sel({ settings: { buildBadgeOverride, showBuildReadout } }),
}))

import { BuildReadout } from '@/components/layout/build-readout'

beforeEach(() => {
  buildBadgeOverride = 'auto'
  showBuildReadout = true
})

const firstLine = (container: HTMLElement) => container.querySelector('div > span')?.textContent

describe('BuildReadout', () => {
  it('renders nothing when the override is off', () => {
    buildBadgeOverride = 'off'
    const { container } = render(<BuildReadout />)
    expect(container).toBeEmptyDOMElement()
  })

  it('reads `channel - time` for dev and nightly, never a version', () => {
    for (const channel of ['dev', 'nightly']) {
      buildBadgeOverride = channel
      const { container, unmount } = render(<BuildReadout />)
      expect(firstLine(container)).toMatch(new RegExp(`^${channel} - \\d\\d-\\d\\d \\d\\d:\\d\\d$`))
      unmount()
    }
  })

  it('reads `channel - version` for beta', () => {
    buildBadgeOverride = 'beta'
    const { container } = render(<BuildReadout />)
    expect(firstLine(container)).toBe('beta - 0.0.0-beta.1')
  })

  it('reads only the version for release', () => {
    buildBadgeOverride = 'release'
    const { container } = render(<BuildReadout />)
    expect(firstLine(container)).toBe('0.0.0')
  })

  it('puts the frame stats on a second line', () => {
    buildBadgeOverride = 'beta'
    const { container } = render(<BuildReadout />)
    expect(container.textContent).toBe('beta - 0.0.0-beta.1fps: --max: --drops: 0')
  })

  it('renders nothing when the build readout is turned off', () => {
    showBuildReadout = false
    const { container } = render(<BuildReadout />)
    expect(container).toBeEmptyDOMElement()
  })

  it('is hidden from assistive tech, being decoration', () => {
    const { container } = render(<BuildReadout />)
    const readout = container.firstElementChild as HTMLElement
    expect(readout).toHaveAttribute('aria-hidden', 'true')
  })
})

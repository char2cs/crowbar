import { act, render } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { reportChannelState, resetConnectionState } from '@/lib/ws/connection-store'

let buildBadgeOverride: string = 'auto'
let showBuildReadout = true
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) =>
    sel({ settings: { buildBadgeOverride, showBuildReadout } }),
}))

import { BuildReadout } from '@/components/layout/build-readout'

beforeEach(() => {
  buildBadgeOverride = 'beta'
  showBuildReadout = true
  resetConnectionState()
})

const lines = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('div > span'), (el) => el.textContent)

const netState = (container: HTMLElement) =>
  container.querySelector('[data-net-state]')?.getAttribute('data-net-state')

describe('BuildReadout', () => {
  it('renders nothing when the override is off', () => {
    buildBadgeOverride = 'off'
    const { container } = render(<BuildReadout />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the build readout is turned off', () => {
    showBuildReadout = false
    const { container } = render(<BuildReadout />)
    expect(container).toBeEmptyDOMElement()
  })

  it('reads the build and net state over the frame stats and counts', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    const { container } = render(<BuildReadout />)
    expect(lines(container)).toEqual(['beta 0.0.0-beta.1  net ok', '--fps --ms  pty0 ag0 ws0'])
    expect(netState(container)).toBe('ok')
  })

  it('marks the net segment DOWN and drops the stale counts when the backend is lost', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    const { container } = render(<BuildReadout />)
    act(() => reportChannelState('/v0/ws/git', false, 'local'))
    expect(netState(container)).toBe('down')
    expect(lines(container)).toEqual(['beta 0.0.0-beta.1  net DOWN', '--fps --ms'])

    act(() => reportChannelState('/v0/ws/git', true, 'local'))
    expect(netState(container)).toBe('ok')
    expect(lines(container)[0]).toBe('beta 0.0.0-beta.1  net ok  rc1')
  })

  it('is hidden from assistive tech, being decoration', () => {
    const { container } = render(<BuildReadout />)
    const readout = container.firstElementChild as HTMLElement
    expect(readout).toHaveAttribute('aria-hidden', 'true')
  })
})

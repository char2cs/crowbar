import { act, render } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { reportChannelState, resetConnectionState } from '@/lib/ws/connection-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

let buildBadgeOverride: string = 'auto'
let showBuildReadout = true
const chrome = vi.hoisted(() => ({ phone: false }))
vi.mock('@/lib/native-chrome', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/native-chrome')>()),
  hasNativeChrome: () => chrome.phone,
}))
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) =>
    sel({ settings: { buildBadgeOverride, showBuildReadout } }),
}))

import { BuildReadout, NativeReadoutView } from '@/components/layout/build-readout'

beforeEach(() => {
  buildBadgeOverride = 'beta'
  showBuildReadout = true
  chrome.phone = false
  resetConnectionState()
  resetHostWorld()
})

afterEach(() => {
  destroyHostRuntime('h1')
  destroyHostRuntime('h2')
})

const setStatus = (hostId: string, status: HostStatus) =>
  act(() => getOrCreateHostRuntime(hostId).getState().setStatus(status))

const hostStates = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('[data-host-net]'), (el) =>
    el.getAttribute('data-host-net'),
  )

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

  it('leaves the phone’s readout to its sidebar page', () => {
    chrome.phone = true
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

  it('folds each paired remote host into the net segment, after the local reading', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    pairRemoteHost('h1', 'homelab-proxmox')
    pairRemoteHost('h2', 'studio')
    setStatus('h1', 'up')
    setStatus('h2', 'down')
    const { container } = render(<BuildReadout />)

    expect(lines(container)).toEqual([
      'beta 0.0.0-beta.1  net ok  homelab-proxm… ok  studio DOWN',
      '--fps --ms  pty0 ag0 ws0',
    ])
    expect(netState(container)).toBe('ok')
    expect(hostStates(container)).toEqual(['ok', 'down'])

    setStatus('h2', 'needs-repair')
    expect(lines(container)[0]).toBe('beta 0.0.0-beta.1  net ok  homelab-proxm… ok  studio repair')
    expect(hostStates(container)).toEqual(['ok', 'repair'])
  })

  it('keeps the local reading, reconnect count included, ahead of the hosts', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    pairRemoteHost('h1', 'lab')
    setStatus('h1', 'up')
    const { container } = render(<BuildReadout />)
    act(() => reportChannelState('/v0/ws/git', false, 'local'))
    expect(lines(container)).toEqual(['beta 0.0.0-beta.1  net DOWN  lab ok', '--fps --ms'])

    act(() => reportChannelState('/v0/ws/git', true, 'local'))
    expect(lines(container)[0]).toBe('beta 0.0.0-beta.1  net ok  rc1  lab ok')
    expect(netState(container)).toBe('ok')
  })

  it('never adds a line, however many hosts are paired', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    pairRemoteHost('h1', 'a-very-long-host-name-indeed')
    pairRemoteHost('h2', 'studio')
    const { container } = render(<BuildReadout />)

    expect(lines(container)).toHaveLength(2)
    expect(lines(container)[0]).toBe('beta 0.0.0-beta.1  net ok  a-very-long-h… --  studio --')
  })

  it('keeps a local-only install exactly as it was', () => {
    reportChannelState('/v0/ws/git', true, 'local')
    const { container } = render(<BuildReadout />)

    expect(lines(container)).toEqual(['beta 0.0.0-beta.1  net ok', '--fps --ms  pty0 ag0 ws0'])
    expect(hostStates(container)).toEqual([])
  })

  it('is hidden from assistive tech, being decoration', () => {
    const { container } = render(<BuildReadout />)
    const readout = container.firstElementChild as HTMLElement
    expect(readout).toHaveAttribute('aria-hidden', 'true')
  })
})

describe('NativeReadoutView', () => {
  it('sets the shell’s resolved readout the way the desktop sets its own', () => {
    const { container } = render(
      <NativeReadoutView
        readout={{
          lines: {
            build: 'beta 0.0.0',
            net: 'net DOWN',
            reconnects: 'rc1',
            stats: '60fps 8ms',
            netWarn: true,
          },
          net: 'down',
          hosts: [{ name: 'lab', net: 'repair' }],
        }}
      />,
    )
    expect(lines(container)).toEqual(['beta 0.0.0  net DOWN  rc1  lab repair', '60fps 8ms'])
    expect(netState(container)).toBe('down')
    expect(hostStates(container)).toEqual(['repair'])
  })
})

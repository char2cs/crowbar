import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { SpaceHeaderLabel } from '@/components/sidebar/space-header-label'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import {
  reportChannelState,
  resetConnectionState,
  useConnectionStore,
} from '@/lib/ws/connection-store'
import { pairRemoteHost, projectOn, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const label = () => screen.getByTestId('space-header-label')
const dot = (hostId: string) => screen.getByTestId(`host-status-${hostId}`)

function remoteProject(status: HostStatus, hostName = 'homelab-prox') {
  pairRemoteHost('h1', hostName)
  getOrCreateHostRuntime('h1').getState().setStatus(status)
  render(<SpaceHeaderLabel project={projectOn('Crowbar API', 'h1')} />)
}

beforeEach(() => resetHostWorld())

afterEach(() => {
  destroyHostRuntime('h1')
  resetConnectionState()
})

describe('SpaceHeaderLabel', () => {
  it('a local project reads Local, with the local daemon’s own status', () => {
    useConnectionStore.setState({ status: 'connected' })
    render(<SpaceHeaderLabel project={projectOn('Rabbyte Labs', 'local')} />)

    expect(screen.getByText('Rabbyte Labs')).toBeInTheDocument()
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(dot('local')).toHaveAttribute('aria-label', 'Online')
    expect(label()).not.toHaveAttribute('data-unreachable')
  })

  it('a remote project reads its host’s name, online while the host is up', () => {
    remoteProject('up')

    expect(screen.getByText('Crowbar API')).toBeInTheDocument()
    expect(screen.getByText('homelab-prox')).toBeInTheDocument()
    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(dot('h1')).toHaveAttribute('aria-label', 'Online')
    expect(label()).not.toHaveAttribute('data-unreachable')
  })

  it.each([
    ['down', 'Offline'],
    ['needs-repair', 'Needs repair — reconnect this host'],
  ] as const)('a host that is %s keeps its name, says so and dims the row', (status, text) => {
    remoteProject(status)

    expect(screen.getByText('homelab-prox')).toBeInTheDocument()
    expect(dot('h1')).toHaveAttribute('aria-label', text)
    expect(dot('h1')).toHaveAttribute('title', text)
    expect(label()).toHaveAttribute('data-unreachable')
  })

  it.each([
    ['unknown', 'Status unknown'],
    ['probing', 'Checking…'],
  ] as const)('a host that is %s says so without dimming the row', (status, text) => {
    remoteProject(status)

    expect(dot('h1')).toHaveAttribute('aria-label', text)
    expect(label()).not.toHaveAttribute('data-unreachable')
  })

  it('says when the host answers but none of its live streams is open', () => {
    remoteProject('up')
    act(() => reportChannelState('crowbar://h1/v0/projects', false, 'h1'))

    expect(dot('h1')).toHaveAttribute('aria-label', 'Online — no live stream')
    expect(label()).not.toHaveAttribute('data-unreachable')
  })

  // A row whose host is not configured cannot be placed on any daemon: it is never Local.
  it('a project whose host was removed reads not available, never Local', () => {
    pairRemoteHost('h1', 'homelab-prox')
    useHostConfigStore.getState().removeHost('h1')
    render(<SpaceHeaderLabel project={projectOn('Quiver Sync', 'h1')} />)

    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(screen.queryByText('homelab-prox')).not.toBeInTheDocument()
    expect(screen.getByText('Host not available')).toBeInTheDocument()
    expect(dot('h1')).toHaveAttribute('aria-label', 'Host not available')
    expect(label()).toHaveAttribute('data-unreachable')
  })

  it('a host id no configured host has reads not available even on a never-paired install', () => {
    render(<SpaceHeaderLabel project={projectOn('Quiver Sync', 'h9')} />)

    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(dot('h9')).toHaveAttribute('aria-label', 'Host not available')
  })

  it('a status change redraws the dot in place, without remounting the row', () => {
    remoteProject('up')
    const before = label()
    const dotBefore = dot('h1')

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('down'))

    expect(label()).toBe(before)
    expect(dot('h1')).toBe(dotBefore)
    expect(dotBefore).toHaveAttribute('aria-label', 'Offline')
    expect(before).toHaveAttribute('data-unreachable')
  })

  // Truncation itself is layout jsdom does not run: this pins the structure it relies on.
  it('a long host name is whole, on its own line, in its own element after the dot', () => {
    const long = 'homelab-prox-in-the-basement-rack-behind-the-old-nas-and-the-ups'
    remoteProject('up', long)
    const name = screen.getByText(long)

    expect(name.textContent).toBe(long)
    expect(name.previousElementSibling).toBe(dot('h1'))
    expect(screen.getByText('Crowbar API')).not.toContainElement(name)
  })
})

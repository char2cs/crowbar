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
const title = (name = 'Crowbar API') => screen.getByText(name)
const hostLine = () => document.getElementById('host-line')!

function remoteProject(status: HostStatus, hostName = 'homelab-prox') {
  pairRemoteHost('h1', hostName)
  getOrCreateHostRuntime('h1').getState().setStatus(status)
  render(<SpaceHeaderLabel project={projectOn('Crowbar API', 'h1')} hostLineId="host-line" />)
}

/** Both lines a remote project tints, in the state and tone they show. */
const tints = () =>
  [title(), hostLine()].map((el) => [
    el.getAttribute('data-host-state'),
    el.getAttribute('data-host-tone'),
  ])

beforeEach(() => resetHostWorld())

afterEach(() => {
  destroyHostRuntime('h1')
  resetConnectionState()
})

describe('SpaceHeaderLabel', () => {
  it('a local project reads Local, untinted', () => {
    useConnectionStore.setState({ status: 'connected' })
    render(<SpaceHeaderLabel project={projectOn('Rabbyte Labs', 'local')} hostLineId="host-line" />)

    expect(title('Rabbyte Labs')).toBeInTheDocument()
    expect(hostLine()).toHaveTextContent(/^Local$/)
    expect(title('Rabbyte Labs')).not.toHaveAttribute('data-host-state')
    expect(hostLine()).not.toHaveAttribute('data-host-state')
  })

  // Local is the app itself: a dropped local socket (after sleep, say) never paints a
  // never-paired user's project red.
  it('a local project is never tinted, whatever the local daemon’s status', () => {
    useConnectionStore.setState({ status: 'disconnected' })
    render(<SpaceHeaderLabel project={projectOn('Rabbyte Labs', 'local')} hostLineId="host-line" />)

    expect(title('Rabbyte Labs')).not.toHaveAttribute('data-host-tone')
    expect(hostLine()).not.toHaveAttribute('data-host-tone')
  })

  it.each([
    ['up', 'online'],
    ['down', 'offline'],
    ['needs-repair', 'needs-attention'],
    ['unknown', 'unsettled'],
    ['probing', 'unsettled'],
  ] as const)('a remote host that is %s tints the name and host line %s', (status, tone) => {
    remoteProject(status)

    expect(title()).toBeInTheDocument()
    expect(hostLine()).toHaveTextContent('homelab-prox')
    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(tints()).toEqual([
      [status, tone],
      [status, tone],
    ])
  })

  it('a host that answers with none of its live streams open stays online', () => {
    remoteProject('up')
    act(() => reportChannelState('crowbar://h1/v0/projects', false, 'h1'))

    expect(tints()).toEqual([
      ['no-live-stream', 'online'],
      ['no-live-stream', 'online'],
    ])
  })

  // A row whose host is not configured cannot be placed on any daemon: it is never Local.
  it('a project whose host was removed reads not available, never Local, in red', () => {
    pairRemoteHost('h1', 'homelab-prox')
    useHostConfigStore.getState().removeHost('h1')
    render(<SpaceHeaderLabel project={projectOn('Quiver Sync', 'h1')} hostLineId="host-line" />)

    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(screen.queryByText('homelab-prox')).not.toBeInTheDocument()
    expect(hostLine()).toHaveTextContent(/^Host not available$/)
    expect(hostLine()).toHaveAttribute('data-host-tone', 'offline')
    expect(title('Quiver Sync')).toHaveAttribute('data-host-state', 'not-available')
  })

  it('a host id no configured host has reads not available even on a never-paired install', () => {
    render(<SpaceHeaderLabel project={projectOn('Quiver Sync', 'h9')} hostLineId="host-line" />)

    expect(screen.queryByText('Local')).not.toBeInTheDocument()
    expect(hostLine()).toHaveAttribute('data-host-state', 'not-available')
  })

  // The state's mark is the header's host button: the line under the name carries no dot.
  it('gives its host line, by name and with no dot, the id the row’s control is described by', () => {
    remoteProject('down')

    expect(hostLine()).toHaveTextContent('homelab-prox')
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(title()).not.toBe(hostLine())
  })

  // What a screen reader hears on the row's control: the host, and its state unless it is up.
  it.each([
    ['up', 'homelab-prox'],
    ['down', 'homelab-prox, Offline'],
    ['needs-repair', 'homelab-prox, Needs pairing again'],
    ['probing', 'homelab-prox, Checking…'],
  ] as const)('a host that is %s is described to the row’s control as %j', (status, heard) => {
    pairRemoteHost('h1', 'homelab-prox')
    getOrCreateHostRuntime('h1').getState().setStatus(status)
    render(
      <>
        <button type="button" aria-describedby="host-line">
          Crowbar API
        </button>
        <SpaceHeaderLabel project={projectOn('Crowbar API', 'h1')} hostLineId="host-line" />
      </>,
    )

    expect(screen.getByRole('button', { name: 'Crowbar API' })).toHaveAccessibleDescription(heard)
  })

  it('a removed host is described once, by what the line already says', () => {
    render(
      <>
        <button type="button" aria-describedby="host-line">
          Quiver Sync
        </button>
        <SpaceHeaderLabel project={projectOn('Quiver Sync', 'h9')} hostLineId="host-line" />
      </>,
    )

    expect(screen.getByRole('button')).toHaveAccessibleDescription('Host not available')
  })

  it('a status change retints in place, without remounting the row', () => {
    remoteProject('up')
    const before = label()
    const titleBefore = title()

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('down'))

    expect(label()).toBe(before)
    expect(title()).toBe(titleBefore)
    expect(titleBefore).toHaveAttribute('data-host-tone', 'offline')
  })

  // Truncation itself is layout jsdom does not run: this pins the structure it relies on.
  it('a long host name is whole, on its own line, apart from the project name', () => {
    const long = 'homelab-prox-in-the-basement-rack-behind-the-old-nas-and-the-ups'
    remoteProject('up', long)
    const name = screen.getByText(long)

    expect(name.textContent).toBe(long)
    expect(name).toBe(hostLine())
    expect(title()).not.toContainElement(name)
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { NavigateFn } from '@tanstack/react-router'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: () => true,
  registerRemoteHost: vi.fn(async () => {}),
  unregisterRemoteHost: vi.fn(),
  forgetHostToken: vi.fn(),
  probeRemoteFingerprint: vi.fn(),
}))
// Every request this tab can make goes through here: a revoke's DELETE, and nothing else.
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))
const toastErrorFn = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

import { apiFetch } from '@/lib/api'
import { unregisterRemoteHost, forgetHostToken } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import {
  reportChannelState,
  resetConnectionState,
  useConnectionStore,
} from '@/lib/ws/connection-store'
import { HostsSettings } from '@/features/settings/components/tabs/hosts-settings'
import { startHostStatus } from '@/features/hosts/lib/host-status'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useRedirectIfProjectMissing } from '@/components/layout/use-redirect-if-project-missing'
import { ApiError } from '@/lib/api'
import { dataOf } from '@/lib/loadable'
import { connectProjectList, useProjectDataStore } from '@/lib/store/projects'
import { pairRemoteHost, projectOn, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

describe('HostsSettings', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
    vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
    vi.mocked(unregisterRemoteHost).mockReset()
    vi.mocked(forgetHostToken).mockReset()
    toastErrorFn.mockReset()
  })

  it('lists Local plus every saved host, each with a live status dot', () => {
    useHostConfigStore.setState({
      hosts: [
        {
          id: 'h1',
          name: 'homelab',
          endpoint: '1.2.3.4:3737',
          pairedFingerprint: 'SHA256:aaaa',
          clientId: 'c1',
        },
      ],
    })
    render(<HostsSettings />)
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('homelab')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-local')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-h1')).toBeInTheDocument()
  })

  // The app probes every host for its whole life; this tab only shows what it found.
  it('shows each host’s status as the app last saw it, and probes nothing itself', () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    getOrCreateHostRuntime('h1').getState().setStatus('down')
    useConnectionStore.setState({ status: 'connected' })

    render(<HostsSettings />)

    expect(screen.getByTestId('host-status-h1')).toHaveAttribute('aria-label', 'Offline')
    expect(screen.getByTestId('host-status-local')).toHaveAttribute('aria-label', 'Online')
    act(() => getOrCreateHostRuntime('h1').getState().setStatus('needs-repair'))
    expect(screen.getByTestId('host-status-h1')).toHaveAttribute(
      'aria-label',
      'Needs repair — reconnect this host',
    )
    expect(apiFetch).not.toHaveBeenCalled()
    destroyHostRuntime('h1')
  })

  // A host can answer its probe while every stream asked of it is refused: what it shows is
  // then not kept current, and the row says so.
  it('says when a host answers but none of its live streams is open', () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    render(<HostsSettings />)
    const dot = screen.getByTestId('host-status-h1')
    const row = dot.closest('[data-testid="host-row"]')!
    expect(dot).toHaveAttribute('aria-label', 'Online')
    expect(row).not.toHaveTextContent('No live stream')

    act(() => reportChannelState('crowbar://h1/v0/projects', false, 'h1'))
    expect(dot).toHaveAttribute('aria-label', 'Online — no live stream')
    expect(row).toHaveTextContent('No live stream')

    act(() => reportChannelState('crowbar://h1/v0/projects', true, 'h1'))
    expect(dot).toHaveAttribute('aria-label', 'Online')
    expect(row).not.toHaveTextContent('No live stream')

    act(() => {
      reportChannelState('crowbar://h1/v0/projects', false, 'h1')
      getHostRuntime('h1')!.getState().setStatus('down')
    })
    expect(dot).toHaveAttribute('aria-label', 'Offline')
    expect(row).not.toHaveTextContent('No live stream')
    resetConnectionState()
    destroyHostRuntime('h1')
  })

  it('does not offer to revoke or remove the Local entry', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    const localRow = screen.getByText('Local').closest('[data-testid="host-row"]')
    expect(localRow?.querySelector('[aria-label*="revoke" i], [aria-label*="remove" i]')).toBeNull()
  })

  it('clicking Add host opens the add-host dialog', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    fireEvent.click(screen.getByRole('button', { name: /add.*host/i }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('clicking revoke calls the daemon DELETE first, then local cleanup, then drops the config row', async () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(useHostConfigStore.getState().hosts).toEqual([]))
    // Targets the host's OWN authed channel (the 4th positional hostId arg) —
    // never the local daemon's, which is the whole point of the amendment.
    expect(apiFetch).toHaveBeenCalledWith('/v0/clients/c1', { method: 'DELETE' }, undefined, 'h1')
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  it('a failed server-side revoke leaves the host config and local token untouched', async () => {
    useHostConfigStore.setState({
      hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }],
    })
    vi.mocked(apiFetch).mockReset().mockRejectedValue(new Error('host unreachable'))
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    expect(unregisterRemoteHost).not.toHaveBeenCalled()
    expect(forgetHostToken).not.toHaveBeenCalled()
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
  })
})

// Revoke as the running app meets it: the project list live, the host's status kept, its
// providers read and one of its projects open.
describe('revoking a host the app is using', () => {
  const navigate = vi.fn()
  const streams: string[] = []
  let stops: Array<() => void> = []
  const held = () =>
    (dataOf(useProjectDataStore.getState().data) ?? []).map((p) => `${p.id}@${p.hostId}`)

  function OpenProject({ projectId }: { projectId: string }) {
    useRedirectIfProjectMissing(projectId, navigate as unknown as NavigateFn)
    return null
  }

  const revoke = () => {
    render(
      <>
        <HostsSettings />
        <OpenProject projectId="p-remote" />
      </>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Revoke h1' }))
  }

  beforeEach(async () => {
    navigate.mockReset()
    streams.length = 0
    toastErrorFn.mockReset()
    vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
    vi.mocked(unregisterRemoteHost).mockReset()
    vi.mocked(forgetHostToken).mockReset()
    resetHostWorld()
    connectProjectList({
      read: async (hostId) =>
        hostId === 'local' ? [projectOn('p-local', 'local')] : [projectOn('p-remote', hostId)],
      subscribe: (hostId) => {
        streams.push(`open ${hostId}`)
        return () => streams.push(`close ${hostId}`)
      },
    })
    pairRemoteHost('h1')
    stops = [useProjectDataStore.getState().startSync(), startHostStatus()]
    await useProjectDataStore.getState().fetch()
    await waitFor(() => expect(held()).toEqual(['p-local@local', 'p-remote@h1']))
    useAgentProvidersStore.setState({ remote: { h1: [] } })
  })

  afterEach(() => {
    stops.forEach((stop) => stop())
    destroyHostRuntime('h1')
    useAgentProvidersStore.setState({ remote: {} })
    resetHostWorld()
  })

  it('takes the host’s rows, stream, status and providers with it, and leaves its project', async () => {
    expect(streams).toEqual(['open local', 'open h1'])

    revoke()

    await waitFor(() => expect(useHostConfigStore.getState().hosts).toEqual([]))
    expect(held()).toEqual(['p-local@local'])
    expect(streams).toEqual(['open local', 'open h1', 'close h1'])
    expect(getHostRuntime('h1')).toBeUndefined()
    expect(useAgentProvidersStore.getState().remote).toEqual({})
    await waitFor(() => expect(navigate).toHaveBeenCalledWith({ to: '/' }))
  })

  it('a revoke that cannot reach the host changes nothing here, and says why', async () => {
    vi.mocked(apiFetch).mockImplementation(async (path) => {
      if (String(path).startsWith('/v0/clients/')) {
        throw new ApiError('the remote host is unreachable', 503, 'host_unreachable', null, 'h1')
      }
    })

    revoke()

    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    expect(useHostConfigStore.getState().hosts.map((h) => h.id)).toEqual(['h1'])
    expect(held()).toEqual(['p-local@local', 'p-remote@h1'])
    expect(streams).toEqual(['open local', 'open h1'])
    expect(getHostRuntime('h1')?.getState().status).toBe('up')
    expect(useAgentProvidersStore.getState().remote).toEqual({ h1: [] })
    expect(unregisterRemoteHost).not.toHaveBeenCalled()
    expect(forgetHostToken).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
  })
})

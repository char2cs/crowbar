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
const toastInfoFn = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    error: (...a: unknown[]) => toastErrorFn(...a),
    info: (...a: unknown[]) => toastInfoFn(...a),
  },
}))

import { apiFetch } from '@/lib/api'
import { unregisterRemoteHost, forgetHostToken, probeRemoteFingerprint } from '@/lib/crowbar-bridge'
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
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { proxyMarkerError } from '@/lib/api/errors'
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

  // A fingerprint confirmed in one attempt must never carry into the next, even one opened
  // while the last is still closing.
  it('opens a fresh add-host attempt each time, before the last has finished closing', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<HostsSettings />)
    const addHost = screen.getByRole('button', { name: /add.*host/i })
    fireEvent.click(addHost)
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue/i }))
    fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
    expect(screen.getByLabelText(/pairing code/i)).toBeInTheDocument()

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Close' }))
      fireEvent.click(addHost)
    })

    expect(screen.queryByLabelText(/pairing code/i)).toBeNull()
    expect(screen.getByLabelText(/name/i)).toHaveValue('')
    expect(screen.queryByText('SHA256:aaaa')).toBeNull()
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
      <PrimitiveDialogProvider>
        <HostsSettings />
        <OpenProject projectId="p-remote" />
      </PrimitiveDialogProvider>,
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

  it('a revoke that cannot reach the host asks first, and kept, changes nothing here', async () => {
    vi.mocked(apiFetch).mockImplementation(async (path) => {
      if (String(path).startsWith('/v0/clients/')) {
        throw new ApiError('the remote host is unreachable', 503, 'host_unreachable', null, 'h1')
      }
    })

    revoke()

    expect(await screen.findByText(/stays valid on h1 until it is revoked there/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Keep it' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
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

// Revoke never leaves the user with a host they cannot remove, and never drops a live
// credential silently: what could not be revoked or cleared is said, then the row goes.
describe('revoking a host when something fails', () => {
  const homelab = {
    id: 'h1',
    name: 'homelab',
    endpoint: '10.0.0.2:3737',
    pairedFingerprint: 'x',
    clientId: 'c1',
  }
  const DELETE = ['/v0/clients/c1', { method: 'DELETE' }, undefined, 'h1']
  const deletes = () =>
    vi.mocked(apiFetch).mock.calls.filter((call) => call[0] === '/v0/clients/c1').length
  const hosts = () => useHostConfigStore.getState().hosts.map((h) => h.id)

  function revoke() {
    render(
      <PrimitiveDialogProvider>
        <HostsSettings />
      </PrimitiveDialogProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Revoke homelab' }))
  }

  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [homelab] })
    vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
    vi.mocked(unregisterRemoteHost).mockReset()
    vi.mocked(forgetHostToken).mockReset()
    toastErrorFn.mockReset()
    toastInfoFn.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    useHostConfigStore.setState({ hosts: [] })
  })

  it.each([
    ['the proxy registry', () => vi.mocked(unregisterRemoteHost)],
    ['the keychain', () => vi.mocked(forgetHostToken)],
  ])('removes a revoked host even when %s cannot be cleared, and says so', async (_, step) => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    step().mockRejectedValueOnce(new Error('keychain locked'))

    revoke()

    await waitFor(() => expect(hosts()).toEqual([]))
    expect(apiFetch).toHaveBeenCalledWith(...DELETE)
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
    expect(toastErrorFn).toHaveBeenCalledExactlyOnceWith(
      expect.stringContaining('Removed homelab'),
      'keychain locked',
    )
    expect(consoleError).toHaveBeenCalled()
  })

  it('removes an unreachable host once the user says to, revoking nothing twice', async () => {
    vi.mocked(apiFetch).mockRejectedValue(proxyMarkerError('host-unreachable', 'h1'))

    revoke()

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('Can’t reach homelab')
    expect(dialog).toHaveTextContent(/stays valid on homelab until it is revoked there/)
    fireEvent.click(screen.getByRole('button', { name: 'Remove anyway' }))

    await waitFor(() => expect(hosts()).toEqual([]))
    expect(deletes()).toBe(1)
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  it('warns that a host’s identity changed before removing it, and sends it nothing more', async () => {
    vi.mocked(apiFetch).mockRejectedValue(proxyMarkerError('host-identity-changed', 'h1'))

    revoke()

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('homelab’s identity changed')
    expect(dialog).toHaveTextContent('10.0.0.2:3737')
    fireEvent.click(screen.getByRole('button', { name: 'Remove anyway' }))

    await waitFor(() => expect(hosts()).toEqual([]))
    expect(deletes()).toBe(1)
  })

  it('removes a host that already refuses this computer (401) without asking, and says so', async () => {
    vi.mocked(apiFetch).mockRejectedValue(
      new ApiError('client is not paired', 401, undefined, undefined, 'h1'),
    )

    revoke()

    await waitFor(() => expect(hosts()).toEqual([]))
    expect(deletes()).toBe(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(toastInfoFn).toHaveBeenCalledExactlyOnceWith('Removed homelab', expect.any(String))
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  it('keeps a host whose daemon refused the revoke for its own reasons, and says why', async () => {
    vi.mocked(apiFetch).mockRejectedValue(
      new ApiError('client not found', 404, undefined, undefined, 'h1'),
    )

    revoke()

    await waitFor(() =>
      expect(toastErrorFn).toHaveBeenCalledExactlyOnceWith(
        'Could not revoke homelab',
        'client not found',
      ),
    )
    expect(hosts()).toEqual(['h1'])
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(forgetHostToken).not.toHaveBeenCalled()
  })
})

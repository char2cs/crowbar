import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
import type { Host } from '@/features/hosts/types'
import { pairRemoteHost, projectOn, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const homelab: Host = {
  id: 'h1',
  name: 'homelab',
  endpoint: '10.0.0.2:3737',
  pairedFingerprint: 'x',
  clientId: 'c1',
}
const hosts = () => useHostConfigStore.getState().hosts.map((h) => h.id)
const DELETE = ['/v0/clients/c1', { method: 'DELETE' }, undefined, 'h1']
const deletes = () =>
  vi.mocked(apiFetch).mock.calls.filter((call) => call[0] === '/v0/clients/c1').length

function renderSettings(extra?: React.ReactNode) {
  return render(
    <PrimitiveDialogProvider>
      <HostsSettings />
      {extra}
    </PrimitiveDialogProvider>,
  )
}

/** Clicks a host's trash and says yes to the question it asks first. */
async function revoke(name = 'homelab') {
  fireEvent.click(screen.getByRole('button', { name: `Revoke ${name}` }))
  fireEvent.click(await screen.findByRole('button', { name: 'Revoke' }))
}

const dialogGone = () => waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [] })
  vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
  vi.mocked(unregisterRemoteHost).mockReset()
  vi.mocked(forgetHostToken).mockReset()
  toastErrorFn.mockReset()
  toastInfoFn.mockReset()
})

describe('HostsSettings', () => {
  it('lists Local plus every saved host, each with a live status dot', () => {
    useHostConfigStore.setState({ hosts: [homelab] })
    render(<HostsSettings />)
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('homelab')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-local')).toBeInTheDocument()
    expect(screen.getByTestId('host-status-h1')).toBeInTheDocument()
  })

  // The app probes every host for its whole life; this tab only shows what it found.
  it('shows each host’s status as the app last saw it, and probes nothing itself', () => {
    useHostConfigStore.setState({ hosts: [homelab] })
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
    useHostConfigStore.setState({ hosts: [homelab] })
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
    render(<HostsSettings />)
    const localRow = screen.getByText('Local').closest('[data-testid="host-row"]')
    expect(localRow?.querySelector('[aria-label*="revoke" i], [aria-label*="remove" i]')).toBeNull()
  })

  it('Add host refuses a host that is already paired, however its address is typed', async () => {
    useHostConfigStore.setState({ hosts: [homelab] })
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:other')
    render(<HostsSettings />)
    fireEvent.click(screen.getByRole('button', { name: /add.*host/i }))
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'again' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: 'https://10.0.0.2/' } })
    fireEvent.click(screen.getByRole('button', { name: /continue/i }))

    expect(await screen.findByRole('dialog')).toHaveTextContent(
      'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
    )
    expect(screen.queryByRole('button', { name: /confirm fingerprint/i })).toBeNull()
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

  it('a confirmed revoke calls the daemon DELETE first, then local cleanup, then drops the row', async () => {
    useHostConfigStore.setState({ hosts: [homelab] })
    renderSettings()
    await revoke()
    await waitFor(() => expect(hosts()).toEqual([]))
    // Targets the host's OWN authed channel (the 4th positional hostId arg) — never the
    // local daemon's.
    expect(apiFetch).toHaveBeenCalledWith(...DELETE)
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  // A revoke cannot be undone from here: the trash asks first, and the safe answer is the
  // default, so a stray Enter keeps the host.
  it.each(['{Enter}', '{Escape}'])(
    'asks before revoking, starting on Keep it: %s keeps the host and asks it nothing',
    async (key) => {
      useHostConfigStore.setState({ hosts: [homelab] })
      const user = userEvent.setup()
      renderSettings()

      await user.click(screen.getByRole('button', { name: 'Revoke homelab' }))

      const dialog = await screen.findByRole('dialog')
      expect(dialog).toHaveTextContent('Revoke homelab?')
      expect(dialog).toHaveTextContent('To use it again you’ll need to run crowbar pair on it.')
      await waitFor(() => expect(screen.getByRole('button', { name: 'Keep it' })).toHaveFocus())
      await user.keyboard(key)
      await dialogGone()
      expect(apiFetch).not.toHaveBeenCalled()
      expect(hosts()).toEqual(['h1'])
      expect(screen.getByRole('button', { name: 'Revoke homelab' })).toBeEnabled()
    },
  )

  // One DELETE per host while it is out, however many hosts are being revoked at once.
  it('keeps each host’s trash disabled while its own revoke is out', async () => {
    useHostConfigStore.setState({ hosts: [homelab, { ...homelab, id: 'h2', name: 'office' }] })
    vi.mocked(apiFetch).mockReturnValue(new Promise(() => {}))
    renderSettings()

    await revoke('homelab')
    await revoke('office')

    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('button', { name: 'Revoke homelab' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Revoke office' })).toBeDisabled()
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

  async function revokeInUse() {
    renderSettings(<OpenProject projectId="p-remote" />)
    await revoke('h1')
  }

  beforeEach(async () => {
    navigate.mockReset()
    streams.length = 0
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

    await revokeInUse()

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

    await revokeInUse()

    expect(await screen.findByText(/h1 will still list this computer as paired/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Keep it' }))
    await dialogGone()
    expect(hosts()).toEqual(['h1'])
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
  /** Every request goes through the real transport, and the host answers `status` with `body`. */
  async function answerThrough(status: number, body: string) {
    const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
    vi.mocked(apiFetch).mockImplementation(actual.apiFetch)
    const fetch = vi.fn(async () => new Response(body, { status }))
    vi.stubGlobal('fetch', fetch)
    return fetch
  }

  beforeEach(() => useHostConfigStore.setState({ hosts: [homelab] }))

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

    renderSettings()
    await revoke()

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

  // "Remove anyway" deletes the token from this computer, but no tool lists or revokes clients on
  // the host: the dialog says what is true, and promises nothing more.
  it.each([
    ['cannot reach it', proxyMarkerError('host-unreachable', 'h1'), 'Can’t reach homelab'],
    [
      'finds its identity changed',
      proxyMarkerError('host-identity-changed', 'h1'),
      'homelab’s identity changed',
    ],
  ])(
    'asks before removing a host the revoke %s, then revokes nothing twice',
    async (_, err, title) => {
      vi.mocked(apiFetch).mockRejectedValue(err)

      renderSettings()
      await revoke()

      const dialog = await screen.findByRole('dialog')
      expect(dialog).toHaveTextContent(title)
      expect(dialog).toHaveTextContent(
        'This computer forgets its key now, so nothing here can use it.',
      )
      expect(dialog).toHaveTextContent(/will still list this computer as paired/)
      expect(dialog).not.toHaveTextContent(/until it is revoked/)
      fireEvent.click(screen.getByRole('button', { name: 'Remove anyway' }))

      await waitFor(() => expect(hosts()).toEqual([]))
      expect(deletes()).toBe(1)
      expect(forgetHostToken).toHaveBeenCalledWith('h1')
    },
  )

  it('names the endpoint whose identity changed', async () => {
    vi.mocked(apiFetch).mockRejectedValue(proxyMarkerError('host-identity-changed', 'h1'))

    renderSettings()
    await revoke()

    expect(await screen.findByRole('dialog')).toHaveTextContent('10.0.0.2:3737')
  })

  it('removes a host that already refuses this computer (its 401) without asking, and says so', async () => {
    await answerThrough(401, '{"success":false,"error":"client is not paired"}')

    renderSettings()
    await revoke()

    await waitFor(() => expect(hosts()).toEqual([]))
    expect(deletes()).toBe(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(toastInfoFn).toHaveBeenCalledExactlyOnceWith('Removed homelab', expect.any(String))
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  // The proxy's own 401: no key here, so nothing reached the host, which still lists this computer.
  it('asks before removing a host this computer holds no key for, and says it stays paired there', async () => {
    const fetch = await answerThrough(401, 'no stored credential for this host — pair it again')

    renderSettings()
    await revoke()

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('This computer holds no key for homelab')
    expect(dialog).toHaveTextContent('homelab will still list this computer as paired.')
    expect(dialog).not.toHaveTextContent(/nothing left to revoke|forgets its key/)
    fireEvent.click(screen.getByRole('button', { name: 'Keep it' }))
    await dialogGone()
    expect(hosts()).toEqual(['h1'])
    expect(fetch).toHaveBeenCalledOnce()
    expect(toastInfoFn).not.toHaveBeenCalled()
  })

  // Each as the real transport turns it into an error: the proxy's own plain-text answers carry
  // only a status; the daemon's carry its words. A 404 comes from past the bearer check, so
  // this computer's token still works there: it is a failed revoke, not an earlier one.
  it.each([
    ['the keychain refused the token (proxy 500)', 500, 'token lookup failed', 'error (500)'],
    ['the host never answered (proxy 504)', 504, 'request timed out', 'error (504)'],
    ['the connection dropped (proxy 502)', 502, 'crowbar proxy error: reset', 'error (502)'],
    ['the proxy lost the host (502)', 502, 'unknown remote host h1', 'error (502)'],
    [
      'the daemon could not record it (500)',
      500,
      '{"success":false,"error":"database is locked"}',
      'database is locked',
    ],
    [
      'the daemon has no such client (404)',
      404,
      '{"success":false,"error":"client not found"}',
      'client not found',
    ],
  ])(
    'asks before removing a host when %s, with a readable reason',
    async (_, status, body, reason) => {
      const fetch = await answerThrough(status, body)

      renderSettings()
      await revoke()

      const dialog = await screen.findByRole('dialog')
      expect(dialog).toHaveTextContent('Couldn’t revoke homelab')
      const said = reason.startsWith('error') ? `homelab returned an ${reason}` : reason
      expect(dialog).toHaveTextContent(`The revoke failed: ${said}.`)
      expect(dialog).not.toHaveTextContent(/\bh1\b|\b\d{3} \./)
      fireEvent.click(screen.getByRole('button', { name: 'Remove anyway' }))

      await waitFor(() => expect(hosts()).toEqual([]))
      expect(fetch).toHaveBeenCalledOnce()
      expect(forgetHostToken).toHaveBeenCalledWith('h1')
    },
  )

  it('keeps a host whose revoke failed unless told otherwise: Keep it is first, Enter keeps', async () => {
    vi.mocked(apiFetch).mockRejectedValue(
      new ApiError('database is locked', 500, undefined, undefined, 'h1'),
    )
    const user = userEvent.setup()

    renderSettings()
    await revoke()

    await screen.findByRole('button', { name: 'Remove anyway' })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Keep it' })).toHaveFocus())
    await user.keyboard('{Enter}')
    await dialogGone()
    expect(hosts()).toEqual(['h1'])
    expect(deletes()).toBe(1)
    expect(unregisterRemoteHost).not.toHaveBeenCalled()
    expect(forgetHostToken).not.toHaveBeenCalled()
    expect(toastErrorFn).not.toHaveBeenCalled()
  })
})

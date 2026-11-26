import { StrictMode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'

vi.mock('@/lib/api/fetch', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/fetch')>()),
  apiFetch: vi.fn(),
}))

import { apiFetch } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { DevicesSettings } from '@/features/settings/components/tabs/devices-settings'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'

const FINGERPRINT = 'aaaa bbbb cccc dddd eeee ffff'

interface Instance {
  access: { enabled: boolean; address: string; reachable: string[]; fingerprint: string }
  clients: { id: string; name: string; pairedAt: string; lastSeen: string; revokedAt?: string }[]
}

/** One instance as its daemon would answer: what the page shows is only what it was told. */
function answerAs(instances: Record<string, Instance>) {
  vi.mocked(apiFetch).mockImplementation(async (path, init, _retry, hostId = 'local') => {
    const inst = instances[hostId]
    const method = init?.method ?? 'GET'
    if (path === '/v0/access' && method === 'GET') return inst.access
    if (path === '/v0/access') {
      const body = JSON.parse(String(init?.body)) as { enabled: boolean; address?: string }
      inst.access = {
        ...inst.access,
        enabled: body.enabled,
        address: body.address ?? inst.access.address,
        reachable: body.enabled ? [`192.168.1.20:${inst.access.address.split(':')[1]}`] : [],
      }
      return inst.access
    }
    if (path === '/v0/clients' && method === 'GET') return inst.clients
    if (path.startsWith('/v0/clients/') && method === 'DELETE') {
      const id = path.slice('/v0/clients/'.length)
      inst.clients = inst.clients.map((c) => (c.id === id ? { ...c, revokedAt: 'now' } : c))
      return { revoked: true }
    }
    if (path === '/v0/pair/code') {
      return {
        code: '482913',
        serverFingerprint: FINGERPRINT,
        addresses: inst.access.reachable,
        expiresAt: '2026-11-18T10:42:00Z',
      }
    }
    throw new Error(`unexpected ${method} ${path}`)
  })
}

const instance = (clients: Instance['clients'] = [], enabled = false): Instance => ({
  access: {
    enabled,
    address: '0.0.0.0:3737',
    reachable: enabled ? ['192.168.1.20:3737'] : [],
    fingerprint: FINGERPRINT,
  },
  clients,
})

const phone = { id: 'p1', name: 'iPhone', pairedAt: '2026-11-01T00:00:00Z', lastSeen: '' }

function renderTab() {
  return render(
    <PrimitiveDialogProvider>
      <DevicesSettings />
    </PrimitiveDialogProvider>,
  )
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [] })
  treeUiStore.setState({ hostFilter: '' })
  vi.mocked(apiFetch).mockReset()
})

const lab = {
  id: 'lab',
  name: 'homelab',
  endpoint: 'lab.example:3737',
  pairedFingerprint: 'x',
  clientId: 'me',
}

async function pickInstance(label: string) {
  await userEvent.click(await screen.findByRole('combobox'))
  await userEvent.click(await screen.findByRole('option', { name: label }))
}

describe('DevicesSettings', () => {
  it('opens on this computer, not on the sidebar filter’s offline host, and lists the failing instance inline', async () => {
    useHostConfigStore.setState({ hosts: [lab] })
    treeUiStore.setState({ hostFilter: 'lab' })
    vi.mocked(apiFetch).mockImplementation(async (path, _init, _retry, hostId = 'local') => {
      if (hostId === 'lab') throw new Error('homelab is unreachable')
      if (path === '/v0/access') return instance().access
      return []
    })
    renderTab()

    expect(await screen.findByRole('switch', { name: 'Allow device connections' })).toBeVisible()
    expect(screen.queryByRole('alert')).toBeNull()

    await pickInstance('homelab')
    expect(await screen.findByRole('button', { name: 'Try again' })).toBeVisible()
    expect(screen.getByRole('combobox')).toBeVisible()
  })

  it('labels an empty list and says why a device cannot be added yet', async () => {
    answerAs({ local: instance() })
    renderTab()

    expect(await screen.findByText('No devices paired yet.')).toBeVisible()
    expect(screen.getByText('Turn on device connections first.')).toBeVisible()
    expect(screen.queryAllByTestId('device-row')).toHaveLength(0)
  })

  it('is off until the owner turns it on, and then says where it listens', async () => {
    answerAs({ local: instance([phone]) })
    renderTab()

    const toggle = await screen.findByRole('switch', { name: 'Allow device connections' })
    expect(toggle).not.toBeChecked()
    expect(screen.getByRole('button', { name: /add a device/i })).toBeDisabled()

    fireEvent.click(toggle)

    expect(await screen.findByText('Listening on')).toBeVisible()
    expect(screen.getByText('192.168.1.20:3737')).toBeVisible()
    expect(screen.getByRole('button', { name: /add a device/i })).toBeEnabled()
  })

  it('turns on at the address typed beside the port', async () => {
    answerAs({ local: instance() })
    renderTab()
    const field = await screen.findByLabelText('Listen address')

    fireEvent.change(field, { target: { value: '0.0.0.0:4000' } })
    fireEvent.click(screen.getByRole('switch', { name: 'Allow device connections' }))

    await waitFor(() =>
      expect(vi.mocked(apiFetch)).toHaveBeenCalledWith(
        '/v0/access',
        expect.objectContaining({
          body: JSON.stringify({ enabled: true, address: '0.0.0.0:4000' }),
        }),
        undefined,
        'local',
      ),
    )
  })

  it('shows a code with the address and fingerprint to confirm, and when it stops working', async () => {
    answerAs({ local: instance([], true) })
    renderTab()

    fireEvent.click(await screen.findByRole('button', { name: /add a device/i }))

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByLabelText('Pairing code 482913')).toHaveTextContent('482 913')
    expect(within(dialog).getByText('192.168.1.20:3737')).toBeVisible()
    expect(within(dialog).getByText(FINGERPRINT)).toBeVisible()
    expect(within(dialog).getByText(/Works once, until/)).toBeVisible()
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('mints exactly one code per request, so the code shown is the one the daemon holds', async () => {
    answerAs({ local: instance([], true) })
    render(
      <StrictMode>
        <PrimitiveDialogProvider>
          <DevicesSettings />
        </PrimitiveDialogProvider>
      </StrictMode>,
    )

    fireEvent.click(await screen.findByRole('button', { name: /add a device/i }))
    await within(await screen.findByRole('dialog')).findByLabelText('Pairing code 482913')

    const mints = () =>
      vi.mocked(apiFetch).mock.calls.filter(([path]) => path === '/v0/pair/code').length
    expect(mints()).toBe(1)
    fireEvent.click(screen.getByRole('button', { name: 'New code' }))
    await waitFor(() => expect(mints()).toBe(2))
  })

  it('lists a device that paired while the code was showing once the popup is closed', async () => {
    const local = instance([], true)
    answerAs({ local })
    renderTab()

    fireEvent.click(await screen.findByRole('button', { name: /add a device/i }))
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByLabelText('Pairing code 482913')
    local.clients = [phone]
    fireEvent.click(within(dialog).getByRole('button', { name: 'Done' }))

    expect(await screen.findByText('iPhone')).toBeVisible()
  })

  it('revokes a device after asking, and leaves the others paired', async () => {
    const tablet = { ...phone, id: 'p2', name: 'iPad', pairedAt: '2026-10-01T00:00:00Z' }
    answerAs({ local: instance([phone, tablet], true) })
    renderTab()

    fireEvent.click(await screen.findByRole('button', { name: 'Revoke iPhone' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Revoke' }))

    await waitFor(() => expect(screen.queryByText('iPhone')).toBeNull())
    expect(screen.getByText('iPad')).toBeVisible()
  })

  it('turning it off keeps every paired device listed', async () => {
    answerAs({ local: instance([phone], true) })
    renderTab()

    fireEvent.click(await screen.findByRole('switch', { name: 'Allow device connections' }))

    expect(await screen.findByText(/^Off\./)).toBeVisible()
    expect(screen.queryByText('Listening on')).toBeNull()
    expect(screen.getByText('iPhone')).toBeVisible()
  })

  it('reads the remote instance it is pointed at, and cannot revoke the key it connects with', async () => {
    useHostConfigStore.setState({ hosts: [lab] })
    answerAs({
      local: instance(),
      lab: instance([{ ...phone, id: 'me', name: 'This Mac' }, phone], true),
    })
    renderTab()
    await pickInstance('homelab')

    expect(await screen.findByText('This computer')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Revoke This Mac' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Revoke iPhone' })).toBeVisible()
    expect(vi.mocked(apiFetch)).toHaveBeenCalledWith('/v0/clients', undefined, undefined, 'lab')
  })

  it('shows a remote instance’s status read-only, because only its own computer may change it', async () => {
    useHostConfigStore.setState({ hosts: [lab] })
    answerAs({ local: instance(), lab: instance([phone], true) })
    renderTab()
    await pickInstance('homelab')

    expect(await screen.findByText('192.168.1.20:3737')).toBeVisible()
    expect(screen.getByText(/only be changed on the computer homelab runs on/i)).toBeVisible()
    expect(screen.queryByRole('switch', { name: 'Allow device connections' })).toBeNull()
    expect(screen.queryByLabelText('Listen address')).toBeNull()
    expect(screen.getByRole('button', { name: /add a device/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Revoke iPhone' })).toBeVisible()
  })

  it('says why when the instance refuses the change', async () => {
    answerAs({ local: instance() })
    const answer = vi.mocked(apiFetch).getMockImplementation()!
    vi.mocked(apiFetch).mockImplementation(async (path, init, ...rest) => {
      if (path === '/v0/access' && init?.method === 'PUT') {
        throw new ApiError('forbidden', 403, 'forbidden')
      }
      return answer(path, init, ...rest)
    })
    renderTab()

    fireEvent.click(await screen.findByRole('switch', { name: 'Allow device connections' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/only be changed on the computer/i)
    expect(screen.getByRole('switch', { name: 'Allow device connections' })).not.toBeChecked()
  })

  it('words a port another program holds, and leaves the address to edit', async () => {
    answerAs({ local: instance() })
    const answer = vi.mocked(apiFetch).getMockImplementation()!
    vi.mocked(apiFetch).mockImplementation(async (path, init, ...rest) => {
      if (path === '/v0/access' && init?.method === 'PUT') {
        throw new ApiError('That port is already in use. Pick another.', 400, 'invalid_argument')
      }
      return answer(path, init, ...rest)
    })
    renderTab()

    fireEvent.change(await screen.findByLabelText('Listen address'), {
      target: { value: '0.0.0.0:3941' },
    })
    fireEvent.click(screen.getByRole('switch', { name: 'Allow device connections' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'That port is already in use. Pick another.',
    )
    expect(screen.getByLabelText('Listen address')).toBeEnabled()
    expect(screen.getByLabelText('Listen address')).toHaveValue('0.0.0.0:3941')
  })
})

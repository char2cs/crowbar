import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: () => true,
  registerRemoteHost: vi.fn(async () => {}),
  unregisterRemoteHost: vi.fn(),
  forgetHostToken: vi.fn(),
  probeRemoteFingerprint: vi.fn(),
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))

import { ApiError, apiFetch } from '@/lib/api'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { HostsSettings } from '@/features/settings/components/tabs/hosts-settings'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import type { Host } from '@/features/hosts/types'

const homelab: Host = {
  id: 'h1',
  name: 'homelab',
  endpoint: '10.0.0.2:3737',
  pairedFingerprint: 'x',
  clientId: 'c1',
}
const office: Host = { ...homelab, id: 'h2', name: 'office', clientId: 'c2' }

const trash = (name = 'homelab') => screen.getByRole('button', { name: `Revoke ${name}` })
const addHost = () => screen.getByRole('button', { name: 'Add host' })
const dialogGone = () => waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

function renderSettings() {
  return render(
    <PrimitiveDialogProvider>
      <HostsSettings />
    </PrimitiveDialogProvider>,
  )
}

// Keyboard users open the confirm from the trash button, so that is where they must land again.
async function askToRevoke(user: ReturnType<typeof userEvent.setup>, name = 'homelab') {
  trash(name).focus()
  await user.keyboard('{Enter}')
  await screen.findByRole('dialog')
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [homelab, office] })
  vi.mocked(apiFetch).mockReset().mockResolvedValue(undefined)
})

describe('focus while revoking a host', () => {
  it.each(['{Enter}', '{Escape}'])(
    'returns to the trash button when %s keeps the host',
    async (key) => {
      const user = userEvent.setup()
      renderSettings()
      await askToRevoke(user)
      await waitFor(() => expect(screen.getByRole('button', { name: 'Keep it' })).toHaveFocus())

      await user.keyboard(key)

      await dialogGone()
      await waitFor(() => expect(trash()).toHaveFocus())
      expect(apiFetch).not.toHaveBeenCalled()
    },
  )

  it('returns to the trash button when the user keeps a host whose revoke failed', async () => {
    vi.mocked(apiFetch).mockRejectedValue(
      new ApiError('database is locked', 500, undefined, undefined, 'h1'),
    )
    const user = userEvent.setup()
    renderSettings()
    await askToRevoke(user)
    await user.click(screen.getByRole('button', { name: 'Revoke' }))
    await screen.findByRole('button', { name: 'Remove anyway' })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Keep it' })).toHaveFocus())

    await user.keyboard('{Enter}')

    await dialogGone()
    await waitFor(() => expect(trash()).toHaveFocus())
    expect(useHostConfigStore.getState().hosts).toHaveLength(2)
  })

  it('moves to Add host once the revoked host’s row is gone', async () => {
    const user = userEvent.setup()
    renderSettings()
    await askToRevoke(user)

    await user.click(screen.getByRole('button', { name: 'Revoke' }))

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Revoke homelab' })).toBeNull())
    await waitFor(() => expect(addHost()).toHaveFocus())
  })

  // The request may take as long as the host does; the trash stays where a keyboard user can
  // find it, announced as unavailable, and cannot start a second revoke.
  it('keeps a trash focusable but unavailable while its request is out, and asks nothing more of it', async () => {
    vi.mocked(apiFetch).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderSettings()
    await askToRevoke(user)
    await user.click(screen.getByRole('button', { name: 'Revoke' }))
    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(1))

    await waitFor(() => expect(trash()).toHaveAttribute('aria-disabled', 'true'))
    expect(trash()).toBeEnabled()
    expect(trash('office')).not.toHaveAttribute('aria-disabled')
    fireEvent.click(trash())

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })

  it('does not take focus from an Add host dialog opened while the request was out', async () => {
    let answer!: () => void
    vi.mocked(apiFetch).mockReturnValue(new Promise<void>((resolve) => (answer = resolve)))
    const user = userEvent.setup()
    renderSettings()
    await askToRevoke(user)
    await user.click(screen.getByRole('button', { name: 'Revoke' }))
    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(1))
    await user.click(addHost())
    const name = await screen.findByLabelText('Name')
    await user.click(name)

    answer()

    await waitFor(() => expect(useHostConfigStore.getState().hosts).toHaveLength(1))
    expect(name).toHaveFocus()
  })
})

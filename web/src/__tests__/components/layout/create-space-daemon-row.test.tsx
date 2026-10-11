import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const tauri = vi.hoisted(() => ({ on: true }))
vi.mock('@/lib/crowbar-bridge', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/crowbar-bridge')>()),
  isTauri: () => tauri.on,
  probeRemoteFingerprint: vi.fn(),
  pairRemoteHost: vi.fn(),
}))

import { CreateSpaceDaemonRow } from '@/components/layout/create-space-daemon-row'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import type { Host } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { pairRemoteHost, probeRemoteFingerprint } from '@/lib/crowbar-bridge'

function host(over: Partial<Host> & { id: string; name: string }): Host {
  return { endpoint: 'x', pairedFingerprint: 'x', clientId: 'c', ...over }
}

/** The row as Create Space holds it: the selection is the panel's. */
function DaemonRow() {
  const [hostId, setHostId] = useState('local')
  return <CreateSpaceDaemonRow hostId={hostId} onHostIdChange={setHostId} />
}

const daemonTrigger = () => screen.getByRole('combobox', { name: /daemon/i })

/** Waits for the popup's item to become the real, clickable thing (base-ui keeps every item
 *  mounted for measurement but `pointer-events: none`s the popup until `open` propagates). */
/** Opens "+ Add a host…" and asks to pair `address` as `name`. */
async function addHost(user: ReturnType<typeof userEvent.setup>, name: string, address: string) {
  await selectDaemonOption(user, /add a host/i)
  fireEvent.change(screen.getByLabelText(/name/i), { target: { value: name } })
  fireEvent.change(screen.getByLabelText(/address/i), { target: { value: address } })
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
}

async function selectDaemonOption(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  await user.click(daemonTrigger())
  const option = await screen.findByRole('option', { name })
  await waitFor(() => expect(getComputedStyle(option).pointerEvents).not.toBe('none'))
  await user.click(option)
}

beforeEach(() => {
  tauri.on = true
  useHostConfigStore.setState({ hosts: [] })
  useConnectionStore.setState({ status: 'connected' })
})

describe('CreateSpaceDaemonRow', () => {
  // Local by default; a saved host is listed whatever its state, says it in the words its dot
  // (and Settings) use, and is choosable only while up: down, or refusing this client, disabled.
  it.each([
    ['up', false, 'Online'],
    ['down', true, 'Offline'],
    ['needs-repair', true, 'Needs pairing again'],
    ['probing', false, 'Checking…'],
  ] as const)(
    'defaults to Local and lists a %s host, disabled: %s, saying %j',
    async (status, disabled, words) => {
      useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'office-vm' })] })
      getOrCreateHostRuntime('h1').getState().setStatus(status)
      const user = userEvent.setup()
      render(<DaemonRow />)
      expect(daemonTrigger()).toHaveTextContent('Local')

      await user.click(daemonTrigger())
      const option = await screen.findByRole('option', { name: /office-vm/i })
      expect(option.getAttribute('aria-disabled') === 'true').toBe(disabled)
      expect(within(option).getByText(words)).toBeInTheDocument()
      expect(screen.getByRole('option', { name: /local/i })).toHaveTextContent(/^Local$/)
    },
  )

  it("Local's dot says what the app knows of the local daemon, like every other host dot", async () => {
    useConnectionStore.setState({ status: 'disconnected' })
    const user = userEvent.setup()
    render(<DaemonRow />)

    expect(within(daemonTrigger()).getByTestId('host-status-local')).toHaveAccessibleName('Offline')
    await user.click(daemonTrigger())
    const local = await screen.findByRole('option', { name: /local/i })
    expect(within(local).getByTestId('host-status-local')).toHaveAccessibleName('Offline')
  })

  it('picking "+ Add a host…" opens AddHostDialog and leaves the daemon selection on Local', async () => {
    const user = userEvent.setup()
    render(<DaemonRow />)

    await selectDaemonOption(user, /add a host/i)

    expect(screen.getByRole('dialog', { name: /add remote host/i })).toBeInTheDocument()
    // The open dialog hides the rest of the page from the accessibility tree: close it before
    // checking the trigger never picked up the sentinel value as a real selection.
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(daemonTrigger()).toHaveTextContent('Local')
  })

  // What was typed or confirmed in one add-host attempt must not carry into the next, even one
  // picked while the last is still closing.
  it('opens a fresh add-host attempt each time it is picked, before the last has closed', async () => {
    const user = userEvent.setup()
    render(<DaemonRow />)
    await selectDaemonOption(user, /add a host/i)
    const option = screen.getByRole('option', { name: /add a host/i, hidden: true })
    const hostName = () => within(screen.getByRole('dialog')).getByLabelText(/name/i)
    fireEvent.change(hostName(), { target: { value: 'homelab' } })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
      fireEvent.pointerDown(option, { pointerType: 'mouse', buttons: 1 })
      fireEvent.click(option)
    })

    expect(hostName()).toHaveValue('')
  })

  // Pairing is the desktop shell's: outside it, adding a host could only end in an error.
  it('offers "+ Add a host…" only in the desktop app, and keeps the Daemon row', async () => {
    tauri.on = false
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'office-vm' })] })
    const user = userEvent.setup()
    render(<DaemonRow />)

    await user.click(daemonTrigger())

    expect(await screen.findByRole('option', { name: /local/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /office-vm/i })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /add a host/i })).not.toBeInTheDocument()
  })

  it('a host paired from "+ Add a host…" becomes the selected daemon', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(pairRemoteHost).mockResolvedValueOnce('client-1')
    const user = userEvent.setup()
    render(<DaemonRow />)

    await addHost(user, 'homelab', '1.2.3.4:3737')
    fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
    fireEvent.change(screen.getByLabelText(/pairing code/i), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Pair' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(daemonTrigger()).toHaveTextContent('homelab')
  })

  it('"+ Add a host…" refuses a host that is already paired, as Settings does', async () => {
    useHostConfigStore.setState({
      hosts: [host({ id: 'h1', name: 'office-vm', endpoint: '10.0.0.5:3737' })],
    })
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:bbbb')
    const user = userEvent.setup()
    render(<DaemonRow />)

    await addHost(user, 'again', '10.0.0.5')

    expect(
      await screen.findByText(
        'This address is already paired as office-vm. Remove it in Settings → Hosts to pair it again.',
      ),
    ).toBeVisible()
    expect(screen.queryByRole('button', { name: /confirm fingerprint/i })).toBeNull()
  })
})

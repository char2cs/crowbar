import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { CreateSpaceDaemonRow } from '@/components/layout/create-space-daemon-row'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import type { Host } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'

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
async function selectDaemonOption(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  await user.click(daemonTrigger())
  const option = await screen.findByRole('option', { name })
  await waitFor(() => expect(getComputedStyle(option).pointerEvents).not.toBe('none'))
  await user.click(option)
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [] })
  useConnectionStore.setState({ status: 'connected' })
})

describe('CreateSpaceDaemonRow', () => {
  // Local by default; a saved host is listed whatever its state, and choosable only while up:
  // down, or refusing this client until paired again, it is listed but disabled.
  it.each([
    ['up', false],
    ['down', true],
    ['needs-repair', true],
  ] as const)('defaults to Local and lists a %s host, disabled: %s', async (status, disabled) => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'office-vm' })] })
    getOrCreateHostRuntime('h1').getState().setStatus(status)
    const user = userEvent.setup()
    render(<DaemonRow />)
    expect(daemonTrigger()).toHaveTextContent('Local')

    await user.click(daemonTrigger())
    const option = await screen.findByRole('option', { name: /office-vm/i })
    expect(option.getAttribute('aria-disabled') === 'true').toBe(disabled)
  })

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
})

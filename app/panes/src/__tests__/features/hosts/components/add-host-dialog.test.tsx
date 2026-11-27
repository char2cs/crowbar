import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/features/hosts/lib/pairing', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/hosts/lib/pairing')>()),
  redeemPairingCode: vi.fn(),
}))
vi.mock('@/lib/crowbar-bridge', () => ({ probeRemoteFingerprint: vi.fn() }))
vi.mock('@/features/hosts/lib/device-access', () => ({ localDaemonFingerprint: vi.fn() }))

import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { localDaemonFingerprint } from '@/features/hosts/lib/device-access'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

function fillAddressStage(address = '1.2.3.4:3737') {
  fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
  fireEvent.change(screen.getByLabelText(/address/i), { target: { value: address } })
  fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))
}

const codeSlot = () => screen.findByRole('textbox', { name: 'Pairing code' })
const codeField = () => screen.findByRole('group', { name: 'Pairing code' })

describe('AddHostDialog', () => {
  beforeEach(() => {
    vi.mocked(probeRemoteFingerprint).mockReset()
    vi.mocked(redeemPairingCode).mockReset()
    vi.mocked(localDaemonFingerprint).mockReset().mockResolvedValue('')
    useHostConfigStore.setState({ hosts: [] })
  })

  it('says what to run on the host before anything is typed', () => {
    render(<AddHostDialog open onOpenChange={() => {}} />)
    expect(
      screen.getByText(
        'On the host, open Settings → Devices and tap Add a device, or run crowbar pair: either shows the address, fingerprint and code you’ll need.',
      ),
    ).toBeVisible()
    const address = screen.getByPlaceholderText('homelab.example.com or 1.2.3.4')
    expect(address).toHaveAccessibleDescription('Uses port 3737 unless you add one, like :4000.')
  })

  it('stops the name at 64 characters instead of letting a longer one fail at the host', async () => {
    const user = userEvent.setup()
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'n'.repeat(64) } })

    await user.type(screen.getByLabelText(/name/i), 'n'.repeat(6))

    expect(screen.getByLabelText(/name/i)).toHaveValue('n'.repeat(64))
  })

  it('stage 1: entering a name and address and confirming the fingerprint advances to the code stage', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa bbbb cccc dddd eeee ffff')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fillAddressStage()

    await waitFor(() =>
      expect(screen.getByText(/SHA256:aaaa bbbb cccc dddd eeee ffff/i)).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    expect(await codeField()).toBeInTheDocument()
  })

  it('does NOT call redeemPairingCode until the fingerprint has been confirmed', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  it('stage 2: submitting a code calls redeemPairingCode with the confirmed fingerprint', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    fireEvent.change(await codeSlot(), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: /pair|connect/i }))

    await waitFor(() =>
      expect(redeemPairingCode).toHaveBeenCalledWith(
        expect.any(String),
        '1.2.3.4:3737',
        'SHA256:aaaa',
        '123456',
        'homelab',
      ),
    )
  })

  it('retrying a rejected code reuses the confirmed fingerprint without re-probing', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(redeemPairingCode)
      .mockRejectedValueOnce(
        'the host refused pairing (401 Unauthorized): pairing code is not valid',
      )
      .mockResolvedValueOnce(undefined)
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    fireEvent.change(await codeSlot(), { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: /pair|connect/i }))
    await waitFor(() => expect(redeemPairingCode).toHaveBeenCalledTimes(1))
    expect(await screen.findByRole('alert')).toHaveTextContent(/^That code didn’t work\./)

    fireEvent.change(await codeSlot(), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: /pair|connect/i }))

    await waitFor(() => expect(redeemPairingCode).toHaveBeenCalledTimes(2))
    expect(redeemPairingCode).toHaveBeenLastCalledWith(
      expect.any(String),
      '1.2.3.4:3737',
      'SHA256:aaaa',
      '123456',
      'homelab',
    )
    expect(probeRemoteFingerprint).toHaveBeenCalledTimes(1)
  })

  it('still says why a code failed after going Back to the fingerprint', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(redeemPairingCode).mockRejectedValueOnce(
      'the host refused pairing (401 Unauthorized): pairing code is not valid',
    )
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
    fireEvent.change(await codeSlot(), { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Pair' }))
    await screen.findByRole('alert')

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(screen.getByRole('alert')).toHaveTextContent(/^That code didn’t work\./)
  })

  it('opens afresh after it was closed mid-attempt', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    const { rerender } = render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))
    expect(await codeField()).toBeInTheDocument()

    rerender(<AddHostDialog open={false} onOpenChange={() => {}} />)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    rerender(<AddHostDialog open onOpenChange={() => {}} />)

    expect(await screen.findByLabelText(/name/i)).toHaveValue('')
    expect(screen.getByLabelText(/address/i)).toHaveValue('')
    expect(screen.queryByRole('group', { name: 'Pairing code' })).toBeNull()
    expect(screen.queryByText(/SHA256:aaaa/i)).toBeNull()
  })

  it('cannot be dismissed while the host is still answering', async () => {
    let answer!: (fingerprint: string) => void
    vi.mocked(probeRemoteFingerprint).mockReturnValueOnce(new Promise((r) => (answer = r)))
    const onOpenChange = vi.fn()
    render(<AddHostDialog open onOpenChange={onOpenChange} />)
    fillAddressStage()
    await screen.findByRole('button', { name: /connecting/i })

    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(onOpenChange).not.toHaveBeenCalled()

    answer('SHA256:aaaa')
    await screen.findByText(/SHA256:aaaa/i)
    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it.each([
    [
      '1.2.3.4:3737',
      'cannot reach 1.2.3.4:3737: Connection refused (os error 61)',
      'Can’t reach 1.2.3.4:3737. Check the address and port, and that the host is running.',
    ],
    [
      'homelab.example.com',
      'cannot resolve homelab.example.com: failed to lookup address information: nodename nor servname provided, or not known (os error 8)',
      'Couldn’t find homelab.example.com. Check the spelling, or use its IP address.',
    ],
  ])(
    'a probe of %s that fails says what to check and does not advance',
    async (typed, raw, text) => {
      vi.mocked(probeRemoteFingerprint).mockRejectedValueOnce(raw)
      render(<AddHostDialog open onOpenChange={() => {}} />)
      fillAddressStage(typed)

      expect(await screen.findByRole('alert')).toHaveTextContent(text)
      expect(screen.queryByText(/os error|refused|lookup/i)).toBeNull()
      expect(screen.queryByRole('button', { name: /confirm|looks right/i })).not.toBeInTheDocument()
      expect(redeemPairingCode).not.toHaveBeenCalled()
    },
  )

  it('probes and pairs a name typed without a port at its canonical address', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage('  HomeLab.Example.com ')
    fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
    fireEvent.change(await codeSlot(), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Pair' }))

    await waitFor(() =>
      expect(redeemPairingCode).toHaveBeenCalledWith(
        expect.any(String),
        'homelab.example.com:3737',
        'SHA256:aaaa',
        '123456',
        'homelab',
      ),
    )
    expect(probeRemoteFingerprint).toHaveBeenCalledWith('homelab.example.com:3737')
  })

  it('says what an address needs, dialing nothing, when what was typed is not one', async () => {
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage('me@homelab.example.com/v0')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter a host name or IP address, optionally with a port, like homelab.example.com:3737.',
    )
    expect(probeRemoteFingerprint).not.toHaveBeenCalled()
  })

  it('drops an address error once the address is corrected', async () => {
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage('me@homelab.example.com/v0')
    expect(await screen.findByRole('alert')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: 'homelab:3737' } })

    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('stops before the fingerprint when the address is already paired, and says what to do', async () => {
    useHostConfigStore.setState({
      hosts: [
        {
          id: 'h1',
          name: 'homelab',
          endpoint: 'lab.example.com:3737',
          pairedFingerprint: 'SHA256:aaaa',
          clientId: 'c1',
        },
      ],
    })
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:bbbb')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fillAddressStage('https://Lab.Example.com/')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
    )
    expect(screen.queryByText('SHA256:bbbb')).toBeNull()
    expect(screen.getByLabelText(/address/i)).toBeEnabled()
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  it('refuses this computer’s own daemon reached by address, pointing at Local', async () => {
    vi.mocked(localDaemonFingerprint).mockResolvedValue('SHA256:aaaa')
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fillAddressStage('192.168.1.20:3737')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This is this computer, already shown as Local. There is nothing to pair.',
    )
    expect(screen.queryByText('SHA256:aaaa')).toBeNull()
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  it('moves focus to the fingerprint, where Enter confirms nothing, then to the code field', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    const user = userEvent.setup()
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()

    const fingerprint = await screen.findByRole('group', {
      name: 'Confirm this matches the fingerprint shown on that host: SHA256:aaaa',
    })
    await waitFor(() => expect(fingerprint).toHaveFocus())
    await user.keyboard('{Enter}{Enter}')
    expect(screen.queryByRole('group', { name: 'Pairing code' })).toBeNull()

    await user.click(screen.getByRole('button', { name: /confirm fingerprint/i }))
    await waitFor(async () => expect(await codeSlot()).toHaveFocus())
  })
})

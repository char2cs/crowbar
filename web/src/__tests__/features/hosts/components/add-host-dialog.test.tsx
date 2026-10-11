import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('@/features/hosts/lib/pairing', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/hosts/lib/pairing')>()),
  redeemPairingCode: vi.fn(),
}))
vi.mock('@/lib/crowbar-bridge', () => ({ probeRemoteFingerprint: vi.fn() }))

import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

function fillAddressStage() {
  fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
  fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
  fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))
}

describe('AddHostDialog', () => {
  beforeEach(() => {
    vi.mocked(probeRemoteFingerprint).mockReset()
    vi.mocked(redeemPairingCode).mockReset()
    useHostConfigStore.setState({ hosts: [] })
  })

  it('stage 1: entering a name and address and confirming the fingerprint advances to the code stage', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa bbbb cccc dddd eeee ffff')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fillAddressStage()

    await waitFor(() =>
      expect(screen.getByText(/SHA256:aaaa bbbb cccc dddd eeee ffff/i)).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()
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

    fireEvent.change(await screen.findByLabelText(/code/i), { target: { value: '123456' } })
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

  // The TOFU gate this task exists to enforce: an "innocent" refactor that pipes
  // the probe result straight into pairing (skipping the human confirm click)
  // is the single most plausible regression here. Retrying after a rejected code
  // must reuse the SAME confirmed fingerprint rather than re-probing — proving
  // pairRemoteHost/redeemPairingCode can never fire on a value the user never saw.
  it('retrying a rejected code reuses the confirmed fingerprint without re-probing', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    vi.mocked(redeemPairingCode)
      .mockRejectedValueOnce(new Error('bad code'))
      .mockResolvedValueOnce(undefined)
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    const codeInput = await screen.findByLabelText(/code/i)
    fireEvent.change(codeInput, { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: /pair|connect/i }))
    await waitFor(() => expect(redeemPairingCode).toHaveBeenCalledTimes(1))

    fireEvent.change(await screen.findByLabelText(/code/i), { target: { value: '123456' } })
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

  // A closed dialog's attempt is over: its confirmed fingerprint must not carry into the next.
  it('opens afresh after it was closed mid-attempt', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    const { rerender } = render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))
    expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()

    rerender(<AddHostDialog open={false} onOpenChange={() => {}} />)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    rerender(<AddHostDialog open onOpenChange={() => {}} />)

    expect(await screen.findByLabelText(/name/i)).toHaveValue('')
    expect(screen.getByLabelText(/address/i)).toHaveValue('')
    expect(screen.queryByLabelText(/code/i)).toBeNull()
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

  it('a fingerprint probe failure surfaces an error and does not advance to the confirm stage', async () => {
    vi.mocked(probeRemoteFingerprint).mockRejectedValueOnce(new Error('unreachable'))
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()

    await waitFor(() => expect(screen.getByText(/unreachable/i)).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /confirm|looks right/i })).not.toBeInTheDocument()
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  // Two rows for one daemon would make every one of its projects ambiguous.
  it('stops before the fingerprint when the address is already paired, and says what to do', async () => {
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
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:bbbb')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fillAddressStage()

    expect(
      await screen.findByText(
        'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
      ),
    ).toBeVisible()
    expect(screen.queryByText('SHA256:bbbb')).toBeNull()
    expect(screen.getByLabelText(/address/i)).toBeEnabled()
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })
})

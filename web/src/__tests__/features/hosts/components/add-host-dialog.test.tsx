import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

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

// The first slot carries the field's label; autofill and an input method write the whole code there.
const codeSlot = () => screen.findByRole('textbox', { name: 'Pairing code' })
const codeField = () => screen.findByRole('group', { name: 'Pairing code' })
const slotValues = () =>
  screen.getAllByRole('textbox').map((slot) => (slot as HTMLInputElement).value)

// The repo's DataTransfer stub does not map user.paste(string)'s `text` onto `text/plain`.
function pasteText(user: ReturnType<typeof userEvent.setup>, text: string) {
  const data = new DataTransfer()
  data.setData('text/plain', text)
  return user.paste(data)
}

async function reachCodeStage() {
  vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
  render(<AddHostDialog open onOpenChange={() => {}} />)
  fillAddressStage()
  fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
  await waitFor(async () => expect(await codeSlot()).toHaveFocus())
}

describe('AddHostDialog', () => {
  beforeEach(() => {
    vi.mocked(probeRemoteFingerprint).mockReset()
    vi.mocked(redeemPairingCode).mockReset()
    useHostConfigStore.setState({ hosts: [] })
  })

  it('says what to run on the host before anything is typed', () => {
    render(<AddHostDialog open onOpenChange={() => {}} />)
    expect(
      screen.getByText(
        'Run crowbar pair on the host first: it shows the fingerprint and the code you’ll need.',
      ),
    ).toBeVisible()
  })

  // The host refuses a longer name with a bare "Couldn’t pair": the field stops short of it.
  it('stops the name at 64 characters instead of letting a longer one fail at the host', async () => {
    const user = userEvent.setup()
    render(<AddHostDialog open onOpenChange={() => {}} />)

    await user.type(screen.getByLabelText(/name/i), 'n'.repeat(70))

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

  // The TOFU gate this task exists to enforce: an "innocent" refactor that pipes
  // the probe result straight into pairing (skipping the human confirm click)
  // is the single most plausible regression here. Retrying after a rejected code
  // must reuse the SAME confirmed fingerprint rather than re-probing — proving
  // pairRemoteHost/redeemPairingCode can never fire on a value the user never saw.
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

  // A closed dialog's attempt is over: its confirmed fingerprint must not carry into the next.
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

  // The shell rejects with transport words; the dialog says what to do instead.
  it('a fingerprint probe failure says what to check and does not advance to the confirm stage', async () => {
    vi.mocked(probeRemoteFingerprint).mockRejectedValueOnce(
      'cannot reach 1.2.3.4:3737: Connection refused (os error 61)',
    )
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Can’t reach 1.2.3.4:3737. Check the address and port, and that the host is running.',
    )
    expect(screen.queryByText(/os error|refused/i)).toBeNull()
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

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
    )
    expect(screen.queryByText('SHA256:bbbb')).toBeNull()
    expect(screen.getByLabelText(/address/i)).toBeEnabled()
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  // Focus follows the flow, never dropping to the page behind, and lands on the fingerprint
  // itself: a stray or repeated Enter must not confirm one the user has not read.
  it('moves focus to the fingerprint, where Enter confirms nothing, then to the code field', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    const user = userEvent.setup()
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fillAddressStage()

    const fingerprint = await screen.findByRole('group', {
      name: 'Confirm this matches the fingerprint printed by that host’s CLI: SHA256:aaaa',
    })
    await waitFor(() => expect(fingerprint).toHaveFocus())
    await user.keyboard('{Enter}{Enter}')
    expect(screen.queryByRole('group', { name: 'Pairing code' })).toBeNull()

    await user.click(screen.getByRole('button', { name: /confirm fingerprint/i }))
    await waitFor(async () => expect(await codeSlot()).toHaveFocus())
  })

  // A code copied from a terminal often carries a space; the host wants exactly six digits.
  it('a pasted code loses its spaces, and Pair waits for exactly six digits', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    const pair = screen.getByRole('button', { name: 'Pair' })
    expect(await codeSlot()).toHaveAccessibleDescription(
      'The 6-digit code crowbar pair printed on the host.',
    )

    await pasteText(user, ' 123 456\n')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
    expect(pair).not.toHaveAttribute('aria-disabled')

    await user.keyboard('{Backspace}')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', ''])
    expect(pair).toHaveAttribute('aria-disabled', 'true')
    // Merely incomplete is not wrong: the hint stays, and nothing is announced.
    expect(
      screen.getAllByRole('textbox', {
        description: 'The 6-digit code crowbar pair printed on the host.',
      }),
    ).toHaveLength(6)
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
  })

  // A wrong code costs one of ten guesses before the host burns it: only the user's Pair sends one.
  it('redeems the typed digits on Enter, never on the sixth keystroke', async () => {
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    const user = userEvent.setup()
    await reachCodeStage()
    const pair = screen.getByRole('button', { name: 'Pair' })
    expect(pair).toHaveAttribute('aria-disabled', 'true')

    await user.keyboard('12345{Enter}')
    expect(redeemPairingCode).not.toHaveBeenCalled()
    expect(pair).toHaveAttribute('aria-disabled', 'true')

    await user.keyboard('6')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
    expect(pair).not.toHaveAttribute('aria-disabled')
    expect(redeemPairingCode).not.toHaveBeenCalled()

    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(redeemPairingCode).toHaveBeenCalledWith(
        expect.any(String),
        '1.2.3.4:3737',
        'SHA256:aaaa',
        '123456',
        'homelab',
      ),
    )
    expect(redeemPairingCode).toHaveBeenCalledTimes(1)
  })

  it('a pasted six-digit code fills every slot and waits for Pair', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await pasteText(user, '654321')

    expect(slotValues()).toEqual(['6', '5', '4', '3', '2', '1'])
    expect(screen.getByRole('button', { name: 'Pair' })).not.toHaveAttribute('aria-disabled')
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  // Each wrong code costs one of ten guesses: more digits than a code holds is no code at all.
  it('refuses a paste of more than six digits instead of cutting it down to a code', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    const pair = screen.getByRole('button', { name: 'Pair' })

    await pasteText(user, 'code for host 2 is 482913')
    expect(slotValues()).toEqual(['', '', '', '', '', ''])
    expect(pair).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('status')).toHaveTextContent('A pairing code is exactly 6 digits.')

    await pasteText(user, '48')
    expect(slotValues()).toEqual(['4', '8', '', '', '', ''])
    expect(pair).toHaveAttribute('aria-disabled', 'true')
  })

  it('takes the code out of the words pasted around it', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await pasteText(user, 'Code: 482913')

    expect(slotValues()).toEqual(['4', '8', '2', '9', '1', '3'])
    expect(screen.getByRole('button', { name: 'Pair' })).not.toHaveAttribute('aria-disabled')
  })

  // After a refused code focus sits on the last slot; a code pasted there must not splice into it.
  it('refuses a paste that runs past the last slot, then takes it into the emptied field', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    await user.keyboard('123456')
    const pair = screen.getByRole('button', { name: 'Pair' })

    await pasteText(user, '482913')
    expect(slotValues()).toEqual(['', '', '', '', '', ''])
    expect(pair).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('status')).toHaveTextContent('A pairing code is exactly 6 digits.')
    expect(await codeSlot()).toHaveFocus()

    await pasteText(user, '482913')
    expect(slotValues()).toEqual(['4', '8', '2', '9', '1', '3'])
    expect(pair).not.toHaveAttribute('aria-disabled')
  })

  // An input method's text is not entered until committed: rewriting it or moving focus first aborts it.
  it('leaves an input method composing alone and takes its digits once committed', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    await user.keyboard('12')
    const slot = screen.getByRole('textbox', { name: 'Character 3 of 6' })
    expect(slot).toHaveFocus()

    fireEvent.compositionStart(slot)
    fireEvent.input(slot, { target: { value: '３' }, isComposing: true })
    fireEvent.keyDown(slot, { key: 'ArrowLeft', isComposing: true })
    expect(slot).toHaveValue('３')
    expect(slot).toHaveFocus()

    fireEvent.compositionEnd(slot, { data: '３' })
    expect(slotValues()).toEqual(['1', '2', '3', '', '', ''])
    expect(screen.getByRole('textbox', { name: 'Character 4 of 6' })).toHaveFocus()
  })

  it('refuses digits an input method commits past the last slot', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    await user.keyboard('12')
    const slot = screen.getByRole('textbox', { name: 'Character 3 of 6' })

    fireEvent.compositionStart(slot)
    fireEvent.input(slot, { target: { value: '３４５６７' }, isComposing: true })
    fireEvent.compositionEnd(slot, { data: '３４５６７' })

    expect(slotValues()).toEqual(['', '', '', '', '', ''])
    expect(screen.getByRole('button', { name: 'Pair' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('takes digits only, from the keyboard and from a paste', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await user.keyboard('1a2-b3')
    expect(slotValues()).toEqual(['1', '2', '3', '', '', ''])

    await pasteText(user, 'x9y')
    expect(slotValues()).toEqual(['1', '2', '3', '9', '', ''])
    expect(screen.getByRole('button', { name: 'Pair' })).toHaveAttribute('aria-disabled', 'true')
  })

  // An East Asian input method types full-width digits (and spaces) unless switched out of it.
  it('reads a code typed in full-width digits as the digits it is', async () => {
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    await reachCodeStage()

    fireEvent.change(await codeSlot(), { target: { value: '１２３\u3000４５６' } })
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
    fireEvent.click(screen.getByRole('button', { name: 'Pair' }))

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
})

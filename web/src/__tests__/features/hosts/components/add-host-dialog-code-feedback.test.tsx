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

const PLAIN_HINT = 'The 6-digit code shown on the host.'
const CORRECTION = 'A pairing code is exactly 6 digits.'
const WRONG_CODE = 'the host refused pairing (401 Unauthorized): pairing code is not valid'

const firstSlot = () => screen.getByRole('textbox', { name: 'Pairing code' })
const slot = (n: number) => screen.getByRole('textbox', { name: `Character ${n} of 6` })
const pairButton = () => screen.getByRole('button', { name: /^Pair(ing…)?$/ })
// The first slot carries the field's label; autofill and an input method write the whole code there.
const codeSlot = () => screen.findByRole('textbox', { name: 'Pairing code' })
const slotValues = () =>
  screen.getAllByRole('textbox').map((input) => (input as HTMLInputElement).value)

// The repo's DataTransfer stub does not map user.paste(string)'s `text` onto `text/plain`.
function pasteText(user: ReturnType<typeof userEvent.setup>, text: string) {
  const data = new DataTransfer()
  data.setData('text/plain', text)
  return user.paste(data)
}

async function reachCodeStage() {
  vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
  render(<AddHostDialog open onOpenChange={() => {}} />)
  fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
  fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
  fireEvent.click(screen.getByRole('button', { name: /continue/i }))
  fireEvent.click(await screen.findByRole('button', { name: /confirm fingerprint/i }))
  await waitFor(() => expect(firstSlot()).toHaveFocus())
}

beforeEach(() => {
  vi.mocked(probeRemoteFingerprint).mockReset()
  vi.mocked(redeemPairingCode).mockReset()
  useHostConfigStore.setState({ hosts: [] })
})

// A disabled control drops the focus it had to the page behind the dialog.
describe('AddHostDialog while a code is tried', () => {
  it('leaves the user in the code field, which cannot be edited or sent twice meanwhile', async () => {
    let refuse!: (reason: string) => void
    vi.mocked(redeemPairingCode).mockReturnValueOnce(new Promise((_, reject) => (refuse = reject)))
    const user = userEvent.setup()
    await reachCodeStage()

    await user.keyboard('123456{Enter}')

    expect(pairButton()).toHaveAccessibleName('Pairing…')
    expect(pairButton()).toHaveAttribute('aria-busy', 'true')
    expect(slot(6)).toBeEnabled()
    expect(slot(6)).toHaveFocus()
    await user.keyboard('{Backspace}9{Enter}')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
    fireEvent.click(pairButton())
    expect(redeemPairingCode).toHaveBeenCalledTimes(1)

    refuse(WRONG_CODE)
    await screen.findByRole('alert')
    expect(slot(6)).toHaveFocus()
  })

  it('puts the user back in the code field when a code sent with the Pair button is refused', async () => {
    vi.mocked(redeemPairingCode).mockRejectedValueOnce(WRONG_CODE)
    const user = userEvent.setup()
    await reachCodeStage()
    await user.keyboard('000000')

    await user.click(pairButton())

    expect(await screen.findByRole('alert')).toHaveTextContent(/^That code didn’t work\./)
    await waitFor(() => expect(firstSlot()).toHaveFocus())
    await user.keyboard('123456')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
  })
})

describe('AddHostDialog after a code was refused', () => {
  // A message about a code the user has since changed describes a code that is no longer there.
  it('takes the failure away as soon as the code changes', async () => {
    vi.mocked(redeemPairingCode).mockRejectedValueOnce(WRONG_CODE)
    await reachCodeStage()
    fireEvent.change(firstSlot(), { target: { value: '000000' } })
    fireEvent.click(pairButton())
    await screen.findByRole('alert')

    fireEvent.change(firstSlot(), { target: { value: '123456' } })

    expect(screen.queryByRole('alert')).toBeNull()
  })
})

describe('AddHostDialog hint while a code is typed', () => {
  it('keeps the plain hint while the code is merely incomplete', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await user.keyboard('123')

    for (const input of screen.getAllByRole('textbox')) {
      expect(input).toHaveAccessibleDescription(PLAIN_HINT)
    }
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
  })

  // Focus is already on the first slot when the paste is refused, so nothing moves to be read.
  it('announces a refused paste, without changing what the slots are described by', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await pasteText(user, 'code for host 2 is 482913')

    expect(slotValues()).toEqual(['', '', '', '', '', ''])
    expect(screen.getByRole('status')).toHaveTextContent(CORRECTION)
    for (const input of screen.getAllByRole('textbox')) {
      expect(input).toHaveAccessibleDescription(PLAIN_HINT)
    }

    await pasteText(user, '482913')
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
  })

  it('says a code is exactly 6 digits when Pair is attempted before it is complete', async () => {
    const user = userEvent.setup()
    await reachCodeStage()

    await user.keyboard('12345{Enter}')

    expect(redeemPairingCode).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent(CORRECTION)
    expect(pairButton()).toHaveAttribute('aria-disabled', 'true')
    expect(slot(6)).toHaveFocus()

    await user.keyboard('6')
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
    expect(pairButton()).not.toHaveAttribute('aria-disabled')
  })
})

describe('AddHostDialog code entry', () => {
  // A code copied from a terminal often carries a space; the host wants exactly six digits.
  it('a pasted code loses its spaces, and Pair waits for exactly six digits', async () => {
    const user = userEvent.setup()
    await reachCodeStage()
    const pair = screen.getByRole('button', { name: 'Pair' })
    expect(await codeSlot()).toHaveAccessibleDescription('The 6-digit code shown on the host.')

    await pasteText(user, ' 123 456\n')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', '6'])
    expect(pair).not.toHaveAttribute('aria-disabled')

    await user.keyboard('{Backspace}')
    expect(slotValues()).toEqual(['1', '2', '3', '4', '5', ''])
    expect(pair).toHaveAttribute('aria-disabled', 'true')
    // Merely incomplete is not wrong: the hint stays, and nothing is announced.
    expect(
      screen.getAllByRole('textbox', {
        description: 'The 6-digit code shown on the host.',
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

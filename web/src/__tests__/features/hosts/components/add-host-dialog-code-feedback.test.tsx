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

const PLAIN_HINT = 'The 6-digit code crowbar pair printed on the host.'
const CORRECTION = 'A pairing code is exactly 6 digits.'
const WRONG_CODE = 'the host refused pairing (401 Unauthorized): pairing code is not valid'

const firstSlot = () => screen.getByRole('textbox', { name: 'Pairing code' })
const slot = (n: number) => screen.getByRole('textbox', { name: `Character ${n} of 6` })
const pairButton = () => screen.getByRole('button', { name: /^Pair(ing…)?$/ })
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

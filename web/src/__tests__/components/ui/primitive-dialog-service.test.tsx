import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  PrimitiveDialogProvider,
  primitiveConfirm,
  primitivePrompt,
} from '@/components/ui/primitive-dialog-service'

function renderProvider() {
  return render(
    <PrimitiveDialogProvider>
      <div>app</div>
    </PrimitiveDialogProvider>,
  )
}

/** Base UI closes a dialog on Escape by firing onOpenChange(false). */
function pressEscape() {
  fireEvent.keyDown(document.activeElement ?? document.body, {
    key: 'Escape',
    code: 'Escape',
  })
}

describe('primitive dialog service', () => {
  it('resolves confirm with the chosen answer', async () => {
    renderProvider()
    const answer = primitiveConfirm('Discard changes?')

    fireEvent.click(await screen.findByRole('button', { name: 'Confirm' }))

    await expect(answer).resolves.toBe(true)
  })

  it('resolves confirm as false when cancelled', async () => {
    renderProvider()
    const answer = primitiveConfirm('Discard changes?')

    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

    await expect(answer).resolves.toBe(false)
  })

  // The safe answer is the default, whatever the confirm does: a stray Enter never confirms.
  it('a confirm starts on its cancel button, so Enter cancels', async () => {
    const user = userEvent.setup()
    renderProvider()
    const answer = primitiveConfirm('Revoke it?', {
      confirmLabel: 'Revoke',
      cancelLabel: 'Keep it',
      destructive: true,
    })

    await waitFor(() => expect(screen.getByRole('button', { name: 'Keep it' })).toHaveFocus())
    await user.keyboard('{Enter}')

    await expect(answer).resolves.toBe(false)
  })

  it('resolves confirm as false when dismissed with Escape', async () => {
    renderProvider()
    const answer = primitiveConfirm('Discard changes?')
    await screen.findByText('Discard changes?')

    pressEscape()

    await expect(answer).resolves.toBe(false)
  })

  it('does not wedge the queue when a dialog is dismissed with Escape', async () => {
    renderProvider()
    const first = primitiveConfirm('First?')
    await screen.findByText('First?')

    pressEscape()
    await expect(first).resolves.toBe(false)

    // A dismissed dialog must not stay at the head of the queue — the next one has to show.
    const second = primitiveConfirm('Second?')
    await screen.findByText('Second?')

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    await expect(second).resolves.toBe(true)
  })

  it('resolves prompt with null when dismissed, and with the value when submitted', async () => {
    renderProvider()
    const dismissed = primitivePrompt('Name?')
    await screen.findByText('Name?')
    pressEscape()
    await expect(dismissed).resolves.toBeNull()

    const answered = primitivePrompt('Name?', { defaultValue: 'main' })
    const input = await screen.findByDisplayValue('main')
    fireEvent.change(input, { target: { value: 'feature' } })
    fireEvent.click(screen.getByRole('button', { name: 'OK' }))

    await expect(answered).resolves.toBe('feature')
  })

  it('shows queued dialogs one at a time, in order', async () => {
    renderProvider()
    const first = primitiveConfirm('First?')
    const second = primitiveConfirm('Second?')

    await screen.findByText('First?')
    expect(screen.queryByText('Second?')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    await expect(first).resolves.toBe(true)

    await waitFor(() => expect(screen.getByText('Second?')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    await expect(second).resolves.toBe(true)
  })
})

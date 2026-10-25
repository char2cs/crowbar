import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
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

  // The header carries the dialog's gutter: a message outside it touches the dialog's edge.
  it('a confirm shows its message as the dialog description, inside the header', async () => {
    renderProvider()
    const message = "To use it again you'll need to run crowbar pair on it."
    void primitiveConfirm(message, { title: 'Revoke docker lab?' })

    const dialog = await screen.findByRole('dialog', {
      name: 'Revoke docker lab?',
      description: message,
    })

    expect(within(dialog).getByText(message).closest('[data-slot="dialog-header"]')).not.toBeNull()
  })

  it('a prompt labels its input with the message', async () => {
    renderProvider()
    void primitivePrompt('Enter a terminal name:', { defaultValue: 'main' })

    expect(await screen.findByRole('textbox', { name: 'Enter a terminal name:' })).toHaveValue(
      'main',
    )
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

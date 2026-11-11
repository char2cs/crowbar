import { fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { showChoice as show } from '@/__tests__/__fixtures__/agent-choice'

const { answerChoiceFn } = vi.hoisted(() => ({ answerChoiceFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
}))

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
})

const elicitation = (properties: object, required: string[] = []) => ({
  kind: 'elicitation',
  toolName: '',
  question: 'The server needs a few details.',
  title: 'deploy-mcp',
  options: [],
  schema: JSON.stringify({ type: 'object', properties, required }),
})

describe('ComposerChoice elicitation form', () => {
  const form = elicitation(
    {
      name: { type: 'string', title: 'Name' },
      region: { type: 'string', enum: ['us', 'eu'] },
      notify: { type: 'boolean' },
    },
    ['name'],
  )

  it('shows the message and one field per property, with nothing invented', () => {
    show(form)

    expect(screen.getByText('The server needs a few details.')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /Name/ })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'eu' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'notify' })).toBeInTheDocument()
    expect(screen.queryByTestId('agent-choice-schema')).toBeNull()
    expect(screen.queryByTestId('agent-choice-detail')).toBeNull()
  })

  it('keeps Submit off, saying why, until the required field is filled', () => {
    show(form)

    expect(screen.getByRole('button', { name: 'Submit' })).toBeDisabled()
    expect(screen.getByText('name is required')).toBeInTheDocument()
  })

  it('marks a field wrong once it is left, not before', () => {
    show(form)
    const name = screen.getByRole('textbox', { name: /Name/ })

    expect(name).not.toHaveAttribute('aria-invalid')
    fireEvent.blur(name)
    expect(name).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getAllByText('name is required').length).toBeGreaterThan(1)
  })

  it('submits typed values as an accept', async () => {
    show(form)

    fireEvent.change(screen.getByRole('textbox', { name: /Name/ }), { target: { value: 'ada' } })
    fireEvent.click(screen.getByRole('radio', { name: 'eu' }))
    fireEvent.click(screen.getByRole('switch', { name: 'notify' }))
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['accept'],
        content: { name: 'ada', region: 'eu', notify: true },
      }),
    )
  })

  it('declines without a form being filled, Decline before Submit', async () => {
    show(form)

    const labels = screen.getAllByRole('button').map((button) => button.textContent)
    expect(labels.indexOf('Decline')).toBeLessThan(labels.indexOf('Submit'))
    fireEvent.click(screen.getByRole('button', { name: 'Decline' }))
    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['decline'] }),
    )
  })
})

describe('ComposerChoice elicitation that cannot be built', () => {
  const unbuildable = elicitation({ nested: { type: 'object' } })

  it('offers Decline and the terminal, never a Submit', () => {
    const onOpenTerminal = vi.fn()
    show(unbuildable, { onOpenTerminal })

    expect(screen.queryByRole('button', { name: 'Submit' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Accept' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-form')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Terminal' }))
    expect(onOpenTerminal).toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Decline' })).toBeEnabled()
  })

  it('declines with the provider’s verb as the option id', async () => {
    show(unbuildable)

    fireEvent.click(screen.getByRole('button', { name: 'Decline' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['decline'] }),
    )
  })
})

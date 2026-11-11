import { fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest'

import { ApiError } from '@/lib/errors/api-error'
import { proxyMarkerError } from '@/lib/api/errors'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { showChoice as show } from '@/__tests__/__fixtures__/agent-choice'

const { answerChoiceFn } = vi.hoisted(() => ({ answerChoiceFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
}))

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
})

describe('ComposerChoice permission card', () => {
  it('sends the option that is picked and waits for the server to confirm it', async () => {
    show()

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('true')
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() => expect(screen.getByTestId('agent-choice-sent')).toBeInTheDocument())
    expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['allow'] })
    expect(screen.getByTestId('agent-choice-prompt')).toBeInTheDocument()
  })

  it('has no text field and no send button: it is the request, not an input', () => {
    show()

    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('button', { name: /send/i })).toBeNull()
    expect(screen.queryByTestId('chat-permission-level-trigger')).toBeNull()
  })

  it('puts Deny at the left and the primary Allow at the right', () => {
    show({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow once' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Allow for session' },
        { id: 'deny', kind: 'deny', label: 'Deny' },
      ],
    })

    const labels = screen
      .getAllByRole('button')
      .map((button) => button.textContent?.replace(/\d$/, ''))
    expect(labels).toEqual(['Deny', 'Allow for session', 'Allow once'])
  })

  it('offers each suggestion as a button that grants it', async () => {
    show({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'deny', kind: 'deny', label: 'Deny' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Allow edits this session' },
      ],
    })

    fireEvent.click(screen.getByRole('button', { name: /Allow edits this session/ }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['suggestion-0'],
      }),
    )
  })

  it('shows only the command for a command request', () => {
    show(
      {},
      {
        request: { toolName: 'Bash', toolInput: { command: 'rm -rf dist', description: 'Clean' } },
      },
    )

    const body = screen.getByTestId('agent-choice-request')
    expect(body).toHaveTextContent('rm -rf dist')
    expect(body).not.toHaveTextContent('Clean')
  })

  it('says how many lines are not shown, beside the answer, until shown', () => {
    const command = Array.from({ length: 20 }, (_, i) => `step ${i}`).join('\n')
    show({}, { request: { toolName: 'Bash', toolInput: { command } } })
    const body = screen.getByTestId('agent-choice-request')
    expect(body).toHaveTextContent('step 11')
    expect(body).not.toHaveTextContent('step 12')
    expect(screen.getByTestId('agent-choice-hidden-lines')).toHaveTextContent(
      '8 more lines not shown',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Show all 20 lines' }))
    expect(body).toHaveTextContent('step 19')
    expect(screen.queryByTestId('agent-choice-hidden-lines')).toBeNull()
    // The unfolded request scrolls by keyboard: its keys are not answers.
    fireEvent.keyDown(body.querySelector('pre')!, { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

describe('ComposerChoice that cannot be answered from here', () => {
  it('draws the whole question read-only, with no action when there is nowhere to send it', () => {
    show({ answerable: false })

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('false')
    expect(screen.getByText('Run Bash?')).toBeInTheDocument()
    expect(screen.getByTestId('agent-choice-options-readonly')).toHaveTextContent('Allow')
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-options')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
  })

  it('offers the terminal when the chat can open one', () => {
    const onOpenTerminal = vi.fn()
    show({ answerable: false }, { onOpenTerminal })

    fireEvent.click(screen.getByRole('button', { name: 'Terminal' }))
    expect(onOpenTerminal).toHaveBeenCalled()
  })

  it('lists nothing for a prompt with nothing to list', () => {
    show({ kind: 'elicitation', options: [], answerable: false })

    expect(screen.queryByTestId('agent-choice-options-readonly')).toBeNull()
  })
})

describe('ComposerChoice failures', () => {
  // A 400 would only repeat, so the controls come down for the place an answer can land.
  it('takes the controls down on an unsupported shape, offering the terminal instead', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    show({}, { onOpenTerminal: vi.fn() })

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument(),
    )
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-error')).toBeNull()
    expect(screen.getByTestId('agent-choice-prompt')).toBeInTheDocument()
  })

  it('leaves no action at all after an unsupported shape with nowhere to redirect to', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    show()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull())
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-error')).toBeNull()
  })

  // 409: the relay let go between poll and click, so the controls come down at once.
  it('takes the controls down when the gate has already closed', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('no longer answerable', 409))
    show({}, { onOpenTerminal: vi.fn() })

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-options-readonly')).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
  })

  it('still surfaces a failure that isn’t a closed gate or an unsupported shape', async () => {
    answerChoiceFn.mockRejectedValue(new Error('network blip'))
    show()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-error')).toHaveTextContent('network blip'),
    )
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled()
  })

  it('names the remote host an answer could not reach', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    answerChoiceFn.mockRejectedValue(proxyMarkerError('host-unreachable', 'h1'))
    show()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-error')).toHaveTextContent(
        'Couldn’t reach Studio Mac',
      ),
    )
    destroyHostRuntime('h1')
    resetHostWorld()
  })
})

describe('ComposerChoice keys', () => {
  it('picks an option with its number key', async () => {
    show()
    fireEvent.keyDown(screen.getByTestId('agent-choice-prompt'), { key: '2' })
    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['deny'] }),
    )
  })

  it('allows once on Enter only while the card has focus', async () => {
    show()
    fireEvent.keyDown(screen.getByRole('button', { name: /Deny/ }), { key: 'Enter' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
    fireEvent.keyDown(screen.getByTestId('agent-choice-prompt'), { key: 'Enter' })
    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['allow'] }),
    )
  })

  it('answers nothing for a key pressed on a control inside the card', () => {
    show()
    fireEvent.keyDown(screen.getByRole('button', { name: /Deny/ }), { key: '2' })
    const hint = screen.getByRole('button', { name: /Allow/ }).querySelector('kbd')!
    fireEvent.keyDown(hint, { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  it('answers nothing for a held key or an Enter that ends an IME composition', () => {
    show()
    const card = screen.getByTestId('agent-choice-prompt')
    fireEvent.keyDown(card, { key: '1', repeat: true })
    fireEvent.keyDown(card, { key: 'Enter', isComposing: true })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  // With focus on the page a digit is for the card; an Enter may end a message.
  it('takes a digit pressed while nothing has focus, never an Enter', async () => {
    show()
    ;(document.activeElement as HTMLElement | null)?.blur()
    fireEvent.keyDown(document.body, { key: 'Enter' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
    fireEvent.keyDown(document.body, { key: '2' })
    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['deny'] }),
    )
  })

  it('answers nothing for a digit typed into a field elsewhere', () => {
    show()
    const field = document.createElement('input')
    document.body.append(field)
    onTestFinished(() => field.remove())
    field.focus()
    fireEvent.keyDown(field, { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  it('shows which digit answers with which option', () => {
    show()
    expect(screen.getByRole('button', { name: /Allow/ })).toHaveTextContent('1')
    expect(screen.getByRole('button', { name: /Deny/ })).toHaveTextContent('2')
  })

  it('takes focus when it appears and nothing else holds it', () => {
    ;(document.activeElement as HTMLElement | null)?.blur()
    show()
    expect(screen.getByTestId('agent-choice-prompt')).toHaveFocus()
  })

  it('leaves focus where it is when something else holds it', () => {
    const elsewhere = document.createElement('input')
    document.body.append(elsewhere)
    onTestFinished(() => elsewhere.remove())
    elsewhere.focus()
    show()
    expect(elsewhere).toHaveFocus()
  })

  it('answers nothing by key while it cannot be answered here', () => {
    show({ answerable: false })
    fireEvent.keyDown(screen.getByTestId('agent-choice-prompt'), { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  it('answers nothing by key while it is set aside in a carousel', () => {
    show({}, { active: false })
    fireEvent.keyDown(window, { key: '1' })
    fireEvent.keyDown(document.body, { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

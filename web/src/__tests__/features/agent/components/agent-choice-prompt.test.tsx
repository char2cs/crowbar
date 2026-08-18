import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AgentActivity, AgentChoice } from '@/features/agent/api/agent-api'
import {
  AgentChoicePrompt,
  AgentChoicePrompts,
} from '@/features/agent/components/agent-choice-prompt'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'
import { ApiError } from '@/lib/api'

const { answerChoiceFn } = vi.hoisted(() => ({ answerChoiceFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
}))

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
})

function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [
      { id: 'allow', kind: 'allow', label: 'Allow' },
      { id: 'deny', kind: 'deny', label: 'Deny' },
    ],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

function draw(overrides: Partial<AgentChoice> = {}, detail = '') {
  return render(
    <AgentChoicePrompt
      wsId="w1"
      chatId="c1"
      choice={choice(overrides)}
      detail={detail}
      providerLabel="Claude"
    />,
  )
}

describe('AgentChoicePrompts', () => {
  it('draws nothing for a chat that is waiting on nobody', () => {
    const { container } = render(
      <AgentChoicePrompts wsId="w1" chatId="c1" activity={NO_ACTIVITY} providerLabel="Claude" />,
    )

    expect(container).toBeEmptyDOMElement()
  })

  // A prompt stops pending three ways, and only one of them goes through this
  // client. The next poll is what takes it off the screen.
  it('drops a prompt the moment the server stops calling it pending', () => {
    const { container } = render(
      <AgentChoicePrompts
        wsId="w1"
        chatId="c1"
        activity={activity({
          choices: [choice({ pending: false, resolution: 'proceeded' })],
        })}
        providerLabel="Claude"
      />,
    )

    expect(container).toBeEmptyDOMElement()
  })

  it('draws every open prompt, oldest first', () => {
    render(
      <AgentChoicePrompts
        wsId="w1"
        chatId="c1"
        activity={activity({
          choices: [
            choice({ id: 'b', seq: 2, toolName: 'Edit' }),
            choice({ id: 'a', seq: 1, toolName: 'Bash' }),
          ],
        })}
        providerLabel="Claude"
      />,
    )

    const drawn = screen.getAllByTestId('agent-choice-prompt')
    expect(drawn.map((node) => node.dataset.choiceId)).toEqual(['a', 'b'])
  })

  // The target is on the tool call the permission gates, not on the prompt.
  it('puts the gated command under a permission’s headline', () => {
    render(
      <AgentChoicePrompts
        wsId="w1"
        chatId="c1"
        activity={activity({
          choices: [choice()],
          toolCalls: [
            {
              id: 't1',
              turnId: 'turn-1',
              seq: 1,
              name: 'Bash',
              target: 'go test ./...',
              status: 'running',
              hasRequest: false,
              hasResult: false,
              startedAt: 'x',
            },
          ],
        })}
        providerLabel="Claude"
      />,
    )

    expect(screen.getByText('Run Bash?')).toBeInTheDocument()
    expect(screen.getByTestId('agent-choice-detail')).toHaveTextContent('go test ./...')
  })
})

describe('AgentChoicePrompt that can be answered', () => {
  it('offers the options as controls and sends the one that is picked', async () => {
    draw()

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('true')
    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['allow'] }),
    )
  })

  // Sent is NOT resolved. The terminal can decide the same prompt in the same
  // instant, so the card stays until the server says how it actually ended.
  it('says the answer is sent and waits for the server to confirm it', async () => {
    draw()

    fireEvent.click(screen.getByRole('button', { name: 'Deny' }))

    await waitFor(() => expect(screen.getByTestId('agent-choice-sent')).toBeInTheDocument())
    expect(screen.getByTestId('agent-choice-prompt')).toBeInTheDocument()
  })

  it('sends EVERY ticked option of a multi-select question in one answer', async () => {
    draw({
      kind: 'question',
      question: 'Which do you want?',
      multi: true,
      options: [
        { id: 'answer-0', kind: 'answer', label: 'Option A', description: 'the first' },
        { id: 'answer-1', kind: 'answer', label: 'Option B' },
        { id: 'answer-2', kind: 'answer', label: 'Option C' },
      ],
    })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Option A' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Option C' }))
    fireEvent.click(screen.getByRole('button', { name: 'Send answer' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['answer-0', 'answer-2'],
      }),
    )
  })

  it('will not send an empty multi-select answer', () => {
    draw({
      kind: 'question',
      question: 'Which do you want?',
      multi: true,
      options: [{ id: 'answer-0', kind: 'answer', label: 'Option A' }],
    })

    expect(screen.getByRole('button', { name: 'Send answer' })).toBeDisabled()
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

describe('AgentChoicePrompt that cannot be answered from here', () => {
  // pending && !answerable is a REAL question — the CLI is asking it at its own
  // terminal — so it is drawn in full, with nothing on it that would reach nobody.
  it('draws the whole question read-only, and says where to answer it', () => {
    draw({ answerable: false })

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('false')
    expect(screen.getByText('Run Bash?')).toBeInTheDocument()
    expect(screen.getByTestId('agent-choice-options-readonly')).toHaveTextContent('Allow')
    expect(screen.getByTestId('agent-choice-terminal-note')).toHaveTextContent(
      'Answer this in the terminal',
    )
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-options')).toBeNull()
  })

  it('offers the terminal when the chat can open one', () => {
    const onOpenTerminal = vi.fn()
    render(
      <AgentChoicePrompt
        wsId="w1"
        chatId="c1"
        choice={choice({ answerable: false })}
        detail=""
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Open the terminal/ }))
    expect(onOpenTerminal).toHaveBeenCalled()
  })
})

describe('AgentChoicePrompt failures', () => {
  // The backend declares no answer template for claude's permission_suggestions,
  // so a 400 is the honest outcome and it is SAID rather than retried away.
  it('surfaces a 400 in the provider’s own terms', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    draw()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-error')).toHaveTextContent(
        'this provider cannot express that answer',
      ),
    )
    // The prompt is still open — the CLI never got an answer.
    expect(screen.getByTestId('agent-choice-prompt')).toBeInTheDocument()
  })

  // 409 means the relay let go between the poll and the click. The controls come
  // down at once rather than lying until the next read.
  it('takes the controls down when the gate has already closed', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('no longer answerable', 409))
    draw()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-terminal-note')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('agent-choice-error')).toHaveTextContent(
      'can no longer be answered from Crowbar',
    )
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
  })
})

describe('AgentChoicePrompt suggestions', () => {
  // Drawn, because the provider offered them. Unsendable, because Crowbar has no
  // shape for them and one narrowed to a plain allow would grant something else.
  it('shows a suggestion the provider offered and refuses to pretend it works', () => {
    draw({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'deny', kind: 'deny', label: 'Deny' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Always allow in this directory' },
      ],
    })

    const suggestions = screen.getByTestId('agent-choice-suggestions')
    expect(suggestions).toHaveTextContent('Always allow in this directory')
    expect(screen.getByRole('button', { name: 'Always allow in this directory' })).toBeDisabled()
    expect(suggestions).toHaveTextContent('Use the terminal for those')
    // The two real answers are untouched by it.
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled()
  })
})

describe('AgentChoicePrompt elicitation', () => {
  const elicitation = {
    kind: 'elicitation',
    toolName: '',
    question: 'What is the deploy target?',
    title: 'deploy-mcp',
    options: [],
    schema: '{"type":"object","properties":{"target":{"type":"string"}}}',
  } satisfies Partial<AgentChoice>

  // An accept means handing back a filled-in form built from the provider's own
  // schema. Crowbar cannot compose that, so it does not offer a button that would
  // send an empty one — it ships the two verbs it CAN mean and says so.
  it('offers decline and cancel only, and says a form answer needs the terminal', () => {
    draw(elicitation)

    expect(screen.getByRole('button', { name: 'Decline' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Accept' })).toBeNull()
    expect(screen.getByTestId('agent-choice-form-note')).toHaveTextContent(
      'has to be done in the terminal',
    )
  })

  // A prompt with no options is answered with the PROVIDER's own verb: the id is
  // the decision key, because there is no option kind to read.
  it('sends the provider’s verb as the option id', async () => {
    draw(elicitation)

    fireEvent.click(screen.getByRole('button', { name: 'Decline' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['decline'] }),
    )
  })

  it('shows the requested schema verbatim rather than guessing at a form', () => {
    draw(elicitation)

    expect(screen.getByTestId('agent-choice-schema')).toHaveTextContent('"target"')
  })
})

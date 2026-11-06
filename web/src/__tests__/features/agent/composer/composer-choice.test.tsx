import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { ApiError } from '@/lib/errors/api-error'
import { proxyMarkerError } from '@/lib/api/errors'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const { answerChoiceFn, setChatPermissionLevelFn } = vi.hoisted(() => ({
  answerChoiceFn: vi.fn(),
  setChatPermissionLevelFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
  setChatPermissionLevel: (...args: unknown[]) => setChatPermissionLevelFn(...args),
}))
vi.mock('@/features/agent/api/agent-providers-api', () => ({
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
}))

// jsdom has no PointerEvent constructor; base-ui's Select opens/commits via
// PointerEvent, same quirk documented in providers-settings.test.tsx's own
// permission-level picker.
if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
  setChatPermissionLevelFn.mockReset()
  setChatPermissionLevelFn.mockResolvedValue(undefined)
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

type ChoiceProps = Partial<ComponentProps<typeof ComposerChoice>>

function show(overrides: Partial<AgentChoice> = {}, props: ChoiceProps = {}) {
  return render(
    <ComposerChoice
      wsId="w1"
      chatId="c1"
      pending={1}
      choice={choice(overrides)}
      providerLabel="Claude"
      {...props}
    />,
  )
}

/** A permission's target comes from the fold: the ask's own, else its tool call's. */
function draw(overrides: Partial<AgentChoice> = {}, detail = '') {
  const subject = choice(overrides)
  return render(
    <ComposerChoice
      wsId="w1"
      chatId="c1"
      pending={1}
      choice={subject}
      toolTarget={detail}
      providerLabel="Claude"
      permissionLevels={['guarded', 'trusted', 'full-auto']}
    />,
  )
}

describe('ComposerChoice that can be answered', () => {
  it('offers the options as controls and sends the one that is picked', async () => {
    draw()

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('true')
    // A plain permission has nothing the terminal covers and this card doesn't.
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
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
      questions: [
        {
          id: 'q0',
          text: 'Which do you want?',
          multi: true,
          options: [
            { id: 'q0-answer-0', kind: 'answer', label: 'Option A', description: 'the first' },
            { id: 'q0-answer-1', kind: 'answer', label: 'Option B' },
            { id: 'q0-answer-2', kind: 'answer', label: 'Option C' },
          ],
        },
      ],
      options: [],
    })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Option A' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Option C' }))
    fireEvent.click(screen.getByRole('button', { name: 'Send answer' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['q0-answer-0', 'q0-answer-2'],
      }),
    )
  })

  it('will not send an empty multi-select answer', () => {
    draw({
      kind: 'question',
      questions: [
        {
          id: 'q0',
          text: 'Which do you want?',
          multi: true,
          options: [{ id: 'q0-answer-0', kind: 'answer', label: 'Option A' }],
        },
      ],
      options: [],
    })

    expect(screen.getByRole('button', { name: 'Send answer' })).toBeDisabled()
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

describe('ComposerChoice that cannot be answered from here', () => {
  // pending && !answerable is a REAL question asked at the CLI's own terminal, so
  // it is drawn in full with nothing on it that would reach nobody.
  it('draws the whole question read-only, with no action when there is nowhere to send it', () => {
    draw({ answerable: false })

    expect(screen.getByTestId('agent-choice-prompt').dataset.answerable).toBe('false')
    expect(screen.getByText('Run Bash?')).toBeInTheDocument()
    expect(screen.getByTestId('agent-choice-options-readonly')).toHaveTextContent('Allow')
    expect(screen.queryByTestId('agent-choice-terminal-note')).toBeNull()
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

  // options/questions can both be empty for an unanswerable prompt (a form
  // nobody can fill and nothing else asked) — there is nothing to list, and the
  // read-only view draws nothing rather than an empty box.
  it('renders nothing above the bar when there is nothing to list', () => {
    draw({ kind: 'elicitation', options: [], answerable: false })

    expect(screen.queryByTestId('agent-choice-options-readonly')).toBeNull()
  })
})

describe('ComposerChoice failures', () => {
  // A 400 means retrying the same controls would only repeat it, so they come down
  // and the one thing left is the place an answer can still land.
  it('takes the controls down on an unsupported shape, offering the terminal instead', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    const onOpenTerminal = vi.fn()
    show({}, { onOpenTerminal })

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument(),
    )
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-error')).toBeNull()
    // The prompt is still open — the CLI never got an answer.
    expect(screen.getByTestId('agent-choice-prompt')).toBeInTheDocument()
  })

  it('leaves no action at all after an unsupported shape with nowhere to redirect to', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    draw()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull())
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-error')).toBeNull()
  })

  // 409 means the relay let go between the poll and the click. The controls come
  // down at once rather than lying until the next read.
  it('takes the controls down when the gate has already closed', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('no longer answerable', 409))
    const onOpenTerminal = vi.fn()
    show({}, { onOpenTerminal })

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-options-readonly')).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
    expect(screen.queryByTestId('agent-choice-error')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Allow' })).toBeNull()
  })

  // A failure that is neither a closed gate nor an unsupported shape is not one
  // the bar has a structural answer for — it stays on screen, in the server's
  // own words, and the controls stay up because retrying might just work.
  it('still surfaces a failure that isn’t a closed gate or an unsupported shape', async () => {
    answerChoiceFn.mockRejectedValue(new Error('network blip'))
    draw()

    fireEvent.click(screen.getByRole('button', { name: 'Allow' }))

    await waitFor(() =>
      expect(screen.getByTestId('agent-choice-error')).toHaveTextContent('network blip'),
    )
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled()
  })

  it('names the remote host an answer could not reach', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    answerChoiceFn.mockRejectedValue(proxyMarkerError('host-unreachable', 'h1'))
    draw()

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

describe('ComposerChoice suggestions', () => {
  // A provider's broader grant ("allow edits for this session") is an answer of
  // its own: pressing it allows this call and applies the grant.
  it('offers each suggestion as a button that grants it', async () => {
    draw({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'deny', kind: 'deny', label: 'Deny' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Allow edits this session' },
      ],
    })

    fireEvent.click(screen.getByRole('button', { name: /Allow edits this session/ }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', expect.any(String), {
        optionIds: ['suggestion-0'],
      }),
    )
  })

  it('needs no terminal for a permission it can answer whole', () => {
    show(
      {
        options: [
          { id: 'allow', kind: 'allow', label: 'Allow' },
          { id: 'suggestion-0', kind: 'suggestion', label: 'Add a permanent rule for this' },
        ],
      },
      { onOpenTerminal: vi.fn() },
    )

    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
    expect(screen.getByRole('button', { name: /Add a permanent rule for this/ })).toBeEnabled()
  })
})

// One ask can carry several questions; drawing only the first left the agent
// waiting on answers this surface could never send.
describe('ComposerChoice with several questions', () => {
  const threeQuestions = {
    kind: 'question',
    toolName: 'AskUserQuestion',
    options: [],
    questions: [
      {
        id: 'q0',
        text: 'Which language?',
        multi: false,
        options: [
          { id: 'q0-answer-0', kind: 'answer', label: 'Go' },
          { id: 'q0-answer-1', kind: 'answer', label: 'TypeScript' },
        ],
      },
      {
        id: 'q1',
        text: 'Which databases?',
        multi: true,
        options: [
          { id: 'q1-answer-0', kind: 'answer', label: 'SQLite' },
          { id: 'q1-answer-1', kind: 'answer', label: 'Postgres' },
          { id: 'q1-answer-2', kind: 'answer', label: 'Redis' },
        ],
      },
      {
        id: 'q2',
        text: 'Deploy where?',
        multi: false,
        options: [
          { id: 'q2-answer-0', kind: 'answer', label: 'Local' },
          { id: 'q2-answer-1', kind: 'answer', label: 'Cloud' },
        ],
      },
    ],
  } satisfies Partial<AgentChoice>

  it('draws a group for every question, single- or multi-select per its own flag', () => {
    draw(threeQuestions)

    expect(screen.getAllByTestId('agent-choice-question')).toHaveLength(3)
    expect(screen.getByText('The agent has 3 questions')).toBeInTheDocument()
    // Question 1 takes one answer, question 2 takes several: the flag is per
    // question, so one card legitimately holds both control shapes.
    expect(screen.getByRole('radio', { name: 'Go' })).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'SQLite' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Cloud' })).toBeInTheDocument()
  })

  it('will not send until EVERY question has an answer', () => {
    draw(threeQuestions)
    const send = () => screen.getByRole('button', { name: 'Send answers' })

    expect(send()).toBeDisabled()
    fireEvent.click(screen.getByRole('radio', { name: 'Go' }))
    expect(send()).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox', { name: 'SQLite' }))
    expect(send()).toBeDisabled()

    fireEvent.click(screen.getByRole('radio', { name: 'Cloud' }))
    expect(send()).toBeEnabled()
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  it('sends every pick of every question in ONE call', async () => {
    draw(threeQuestions)

    fireEvent.click(screen.getByRole('radio', { name: 'Go' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'SQLite' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Redis' }))
    fireEvent.click(screen.getByRole('radio', { name: 'Cloud' }))
    fireEvent.click(screen.getByRole('button', { name: 'Send answers' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['q0-answer-0', 'q1-answer-0', 'q1-answer-2', 'q2-answer-1'],
      }),
    )
    expect(answerChoiceFn).toHaveBeenCalledTimes(1)
  })

  // A single-answer question replaces its pick rather than accumulating one: two
  // answers to it are refused by the backend, so the control must not be able to
  // produce them.
  it('replaces the pick on a single-answer question', async () => {
    draw(threeQuestions)

    fireEvent.click(screen.getByRole('radio', { name: 'Go' }))
    fireEvent.click(screen.getByRole('radio', { name: 'TypeScript' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'SQLite' }))
    fireEvent.click(screen.getByRole('radio', { name: 'Local' }))
    fireEvent.click(screen.getByRole('button', { name: 'Send answers' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['q0-answer-1', 'q1-answer-0', 'q2-answer-0'],
      }),
    )
  })

  // The pane is retained across chat selection, so a re-render can hand it another
  // prompt; picks carried over would name option ids that prompt never offered.
  it('starts empty when a different prompt is rendered into the same card', () => {
    const { rerender } = draw(threeQuestions)
    fireEvent.click(screen.getByRole('radio', { name: 'Go' }))
    expect(screen.getByRole('radio', { name: 'Go' })).toBeChecked()

    rerender(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        choice={choice({ ...threeQuestions, id: 'k2' })}
        pending={1}
        providerLabel="Claude"
      />,
    )

    expect(screen.getByRole('radio', { name: 'Go' })).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Send answers' })).toBeDisabled()
  })

  // pending && !answerable: the CLI is asking at its own terminal. Every option of
  // every question is still worth reading, and none of it is pressable.
  it('reads out every question’s options when it cannot be answered here', () => {
    draw({ ...threeQuestions, answerable: false })

    const listed = screen.getByTestId('agent-choice-options-readonly')
    expect(listed).toHaveTextContent('Go')
    expect(listed).toHaveTextContent('Redis')
    expect(listed).toHaveTextContent('Cloud')
    expect(screen.queryByRole('radio', { name: 'Go' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Send answers' })).toBeNull()
  })
})

// The graceful fallback, and NOT a migration: a prompt recorded before questions
// were modelled has no `questions` at all, and it is still a single question
// described by the prompt's own text and options.
describe('ComposerChoice for a prompt recorded before questions existed', () => {
  it('draws the prompt-level question as a question of one', async () => {
    draw({
      kind: 'question',
      question: 'Which do you want?',
      multi: false,
      options: [
        { id: 'answer-0', kind: 'answer', label: 'Option A' },
        { id: 'answer-1', kind: 'answer', label: 'Option B' },
      ],
    })

    expect(screen.getAllByTestId('agent-choice-question')).toHaveLength(1)
    fireEvent.click(screen.getByRole('radio', { name: 'Option B' }))
    fireEvent.click(screen.getByRole('button', { name: 'Send answer' }))

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['answer-1'] }),
    )
  })
})

describe('ComposerChoice elicitation', () => {
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
  it('offers decline and cancel only, with no accept button for a form Crowbar cannot fill', () => {
    draw(elicitation)

    expect(screen.getByRole('button', { name: 'Decline' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Accept' })).toBeNull()
    expect(screen.queryByTestId('agent-choice-form-note')).toBeNull()
    // No handler was given to open one, so nothing offers to.
    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
  })

  // Why a form needs the terminal isn't written down any more — the Terminal
  // button sits right beside Decline/Cancel in the same row.
  it('offers a terminal button alongside decline and cancel when the chat can open one', () => {
    const onOpenTerminal = vi.fn()
    show(elicitation, { onOpenTerminal })

    fireEvent.click(screen.getByRole('button', { name: 'Terminal' }))
    expect(onOpenTerminal).toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Decline' })).toBeEnabled()
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

// The per-chat permission-level dial sits beside whatever answer controls show.
describe('ComposerChoice permission-level switcher', () => {
  it('sits beside the answer controls on a real, answerable prompt', () => {
    draw()

    expect(screen.getByTestId('chat-permission-level-trigger')).toBeInTheDocument()
  })

  // terminalOnly needs a real `onOpenTerminal`, or this passes for the wrong reason.
  it('does not appear when the prompt cannot be answered from here', () => {
    const onOpenTerminal = vi.fn()
    show({ answerable: false }, { onOpenTerminal })

    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
    expect(screen.queryByTestId('chat-permission-level-trigger')).toBeNull()
  })

  // Absent capability, absent UI: a level the provider does not declare would 400.
  it('does not appear for a provider whose descriptor declares no levels', () => {
    render(
      <ComposerChoice wsId="w1" chatId="c1" choice={choice()} pending={1} providerLabel="Claude" />,
    )

    expect(screen.queryByTestId('chat-permission-level-trigger')).toBeNull()
  })
})

// The oldest pending choice is shown; a second (a parallel subagent's ask) must
// still be signalled.
describe('ComposerChoice in a stack of pending asks', () => {
  it('counts pending asks as i of n', () => {
    show({}, { pending: 3, position: 2 })
    expect(screen.getByTestId('agent-choice-position')).toHaveTextContent('2/3')
  })

  it('says nothing when it is the only one', () => {
    show()
    expect(screen.queryByTestId('agent-choice-position')).toBeNull()
  })
})

describe('ComposerChoice with a long request', () => {
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
    draw()
    fireEvent.keyDown(screen.getByTestId('chat-permission-level-trigger'), { key: '1' })
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

  // The composer gives way to the card, so focus falls to the page: a digit then
  // is meant for the card. An Enter is not: it may be the end of a message.
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
    field.focus()
    fireEvent.keyDown(field, { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
    field.remove()
  })

  it('shows which digit answers with which option', () => {
    show()
    expect(screen.getByRole('button', { name: /Allow/ })).toHaveTextContent('1')
    expect(screen.getByRole('button', { name: /Deny/ })).toHaveTextContent('2')
  })

  it('answers nothing by key while it cannot be answered here', () => {
    show({ answerable: false })
    fireEvent.keyDown(screen.getByTestId('agent-choice-prompt'), { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

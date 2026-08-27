import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AgentActivity, AgentChoice } from '@/features/agent/api/agent-api'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
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

/** The second line is DERIVED now: a permission's target is read off the tool
 *  call it gates, not handed in, so a test that wants one supplies the call. */
function draw(overrides: Partial<AgentChoice> = {}, detail = '') {
  const subject = choice(overrides)
  const gating: AgentActivity = detail
    ? activity({
        toolCalls: [
          {
            id: 'gated',
            turnId: subject.turnId,
            seq: 1,
            name: subject.toolName ?? 'Bash',
            target: detail,
            status: 'running',
            hasRequest: false,
            hasResult: false,
            startedAt: '2026-08-18T12:00:00Z',
          },
        ],
      })
    : NO_ACTIVITY
  return render(
    <ComposerChoice
      wsId="w1"
      chatId="c1"
      activity={gating}
      choice={subject}
      providerLabel="Claude"
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
  // pending && !answerable is a REAL question — the CLI is asking it at its own
  // terminal — so it is drawn in full, with nothing on it that would reach nobody.
  // Why it can't be answered here isn't said — the bar just offers the one thing
  // that still works, and only when there's a handler to make that button real.
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
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        choice={choice({ answerable: false })}
        activity={NO_ACTIVITY}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

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
  // The backend declares no answer template for claude's permission_suggestions,
  // so a 400 means retrying the same controls would only repeat it. The controls
  // come down and the one thing left is the place an answer can still land —
  // not a paragraph explaining why this one couldn't.
  it('takes the controls down on an unsupported shape, offering the terminal instead', async () => {
    answerChoiceFn.mockRejectedValue(new ApiError('this provider cannot express that answer', 400))
    const onOpenTerminal = vi.fn()
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        activity={NO_ACTIVITY}
        choice={choice()}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

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
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        activity={NO_ACTIVITY}
        choice={choice()}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

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
})

describe('ComposerChoice suggestions', () => {
  // DEFECT 5. These used to be a row of DISABLED BUTTONS, which still reads as a
  // control — something greyed out now that will work in a moment — and there is no
  // declared answer template for a suggestion, so pressing one could only ever
  // produce a 400. A note cannot be pressed, which is how that failure stops being
  // reachable from the UI at all.
  it('writes suggestions down as a plain list, with nothing on them to press', () => {
    draw({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'deny', kind: 'deny', label: 'Deny' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Add a permanent rule for this' },
      ],
    })

    const suggestions = screen.getByTestId('agent-choice-suggestions')
    expect(suggestions).toHaveTextContent('Add a permanent rule for this')
    // Nothing inside the list is interactive — not enabled, not disabled, absent.
    expect(suggestions.querySelectorAll('button, input, [role="button"]')).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Add a permanent rule for this' })).toBeNull()
    // The two real answers are untouched by it.
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled()
  })

  // The reason Crowbar can't send a suggestion isn't written down any more — the
  // Terminal button beside Allow/Deny is the answer to that, when there's a
  // handler to make it real.
  it('offers a terminal button in the bar alongside the real controls', () => {
    const onOpenTerminal = vi.fn()
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        activity={NO_ACTIVITY}
        choice={choice({
          options: [
            { id: 'allow', kind: 'allow', label: 'Allow' },
            { id: 'deny', kind: 'deny', label: 'Deny' },
            { id: 'suggestion-0', kind: 'suggestion', label: 'Add a permanent rule for this' },
          ],
        })}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Terminal' }))
    expect(onOpenTerminal).toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Allow' })).toBeEnabled()
  })

  it('does not offer a terminal button without a handler to open one', () => {
    draw({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'Add a permanent rule for this' },
      ],
    })

    expect(screen.queryByRole('button', { name: 'Terminal' })).toBeNull()
  })

  // The backend never sends a raw provider type name any more, but a stale poll
  // could still be holding one — so nothing here reconstructs a control from a
  // label whatever it says.
  it('never turns a suggestion into a control, whatever it is labelled', () => {
    draw({
      options: [
        { id: 'allow', kind: 'allow', label: 'Allow' },
        { id: 'suggestion-0', kind: 'suggestion', label: 'addRules' },
      ],
    })

    expect(screen.queryByRole('button', { name: 'addRules' })).toBeNull()
    expect(screen.queryByRole('checkbox', { name: 'addRules' })).toBeNull()
  })
})

// DEFECT 4. A user asked claude to "ask me 3 questions at the same time". Claude
// issued ONE AskUserQuestion carrying three, Crowbar drew the first, and answering
// it left claude saying "still waiting on: your answers to questions 2 & 3" — a
// state this surface could never leave.
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

  // The pane this card lives in is RETAINED across chat selection, so a re-render
  // can hand it a different prompt without unmounting it. Picks carried into that
  // prompt would be submitted against option ids it never offered — the partial
  // answer this whole card exists to prevent, arrived at from the other side.
  it('starts empty when a different prompt is rendered into the same card', () => {
    const { rerender } = draw(threeQuestions)
    fireEvent.click(screen.getByRole('radio', { name: 'Go' }))
    expect(screen.getByRole('radio', { name: 'Go' })).toBeChecked()

    rerender(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        choice={choice({ ...threeQuestions, id: 'k2' })}
        activity={NO_ACTIVITY}
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
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        activity={NO_ACTIVITY}
        choice={choice(elicitation)}
        providerLabel="Claude"
        onOpenTerminal={onOpenTerminal}
      />,
    )

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

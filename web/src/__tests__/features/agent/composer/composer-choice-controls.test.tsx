import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { choice, showChoice as show } from '@/__tests__/__fixtures__/agent-choice'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'

const { answerChoiceFn } = vi.hoisted(() => ({ answerChoiceFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
}))

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
})

const single = {
  kind: 'question',
  options: [],
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
} satisfies Partial<AgentChoice>

const three = {
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

const click = (role: string, name: string) => fireEvent.click(screen.getByRole(role, { name }))

describe('ComposerChoice question', () => {
  it('shows the question with each option’s label and description', () => {
    show(single)

    expect(screen.getByText('Which do you want?')).toBeInTheDocument()
    expect(screen.getByText('the first')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Option A' })).toBeInTheDocument()
  })

  it('sends EVERY ticked option of a multi-select question in one answer', async () => {
    show(single)

    click('checkbox', 'Option A')
    click('checkbox', 'Option C')
    click('button', 'Send answer')

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['q0-answer-0', 'q0-answer-2'],
      }),
    )
  })

  it('will not send an empty answer', () => {
    show(single)

    expect(screen.getByRole('button', { name: 'Send answer' })).toBeDisabled()
  })

  it('draws a prompt recorded before questions existed as a question of one', async () => {
    show({
      kind: 'question',
      question: 'Which do you want?',
      options: [
        { id: 'answer-0', kind: 'answer', label: 'Option A' },
        { id: 'answer-1', kind: 'answer', label: 'Option B' },
      ],
    })

    click('radio', 'Option B')
    click('button', 'Send answer')

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', { optionIds: ['answer-1'] }),
    )
  })

  it('reads out every option when it cannot be answered here', () => {
    show({ ...three, answerable: false })

    const listed = screen.getByTestId('agent-choice-options-readonly')
    expect(listed).toHaveTextContent('Go')
    expect(listed).toHaveTextContent('SQLite')
    expect(listed).toHaveTextContent('Cloud')
    expect(screen.queryByRole('radio', { name: 'Go' })).toBeNull()
  })
})

describe('ComposerChoice with several questions', () => {
  it('asks one question at a time and will not go on without a pick', () => {
    show(three)

    expect(screen.getAllByTestId('agent-choice-question')).toHaveLength(1)
    expect(screen.getByText('1 of 3 questions')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    click('radio', 'Go')
    click('button', 'Next')
    expect(screen.getByText('Which databases?')).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: 'Go' })).toBeNull()
  })

  it('sends every pick of every question in ONE call, replacing a single-answer pick', async () => {
    show(three)

    click('radio', 'Go')
    click('radio', 'TypeScript')
    click('button', 'Next')
    click('checkbox', 'SQLite')
    click('checkbox', 'Postgres')
    click('button', 'Next')
    click('radio', 'Cloud')
    expect(answerChoiceFn).not.toHaveBeenCalled()
    click('button', 'Send answers')

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k1', {
        optionIds: ['q0-answer-1', 'q1-answer-0', 'q1-answer-1', 'q2-answer-1'],
      }),
    )
    expect(answerChoiceFn).toHaveBeenCalledTimes(1)
  })

  it('keeps a pick when going back to its question', () => {
    show(three)

    click('radio', 'Go')
    click('button', 'Next')
    click('button', 'Back')

    expect(screen.getByRole('radio', { name: 'Go' })).toBeChecked()
  })

  // A retained pane can get another prompt; old picks would name options it never offered.
  it('starts empty when a different prompt is rendered into the same card', () => {
    const { rerender } = show(three)
    click('radio', 'Go')

    rerender(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        choice={choice({ ...three, id: 'k2' })}
        providerLabel="Claude"
      />,
    )

    expect(screen.getByRole('radio', { name: 'Go' })).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })
})

describe('ComposerChoice question Skip', () => {
  it('is offered only where the carousel has another request to go to', () => {
    const { unmount } = show(single)
    expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
    unmount()

    const onSkip = vi.fn()
    render(
      <ComposerChoice
        wsId="w1"
        chatId="c1"
        choice={choice(single)}
        providerLabel="Claude"
        onSkip={onSkip}
      />,
    )
    click('button', 'Skip')
    expect(onSkip).toHaveBeenCalled()
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

describe('ComposerChoice question digits', () => {
  it('numbers the options and picks, or ticks, the one a digit names', () => {
    show(three)
    const card = screen.getByTestId('agent-choice-prompt')
    expect(screen.getByRole('radio', { name: /Go/ }).closest('.rq-opt')).toHaveTextContent('1')
    fireEvent.keyDown(card, { key: '2' })
    expect(screen.getByRole('radio', { name: /TypeScript/ })).toBeChecked()
    fireEvent.keyDown(card, { key: '1' })
    expect(screen.getByRole('radio', { name: /Go/ })).toBeChecked()

    click('button', 'Next')
    fireEvent.keyDown(card, { key: '2' })
    fireEvent.keyDown(card, { key: '1' })
    fireEvent.keyDown(card, { key: '2' })
    expect(screen.getByRole('checkbox', { name: /SQLite/ })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: /Postgres/ })).not.toBeChecked()
  })

  it('sends nothing for a digit: only the send button answers', () => {
    show(single)
    fireEvent.keyDown(screen.getByTestId('agent-choice-prompt'), { key: '1' })
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })
})

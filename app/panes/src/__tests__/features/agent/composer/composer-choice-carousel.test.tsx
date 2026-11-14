import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AskItem } from '@/features/agent/stores/lib/transcript-items'
import { ComposerChoiceCarousel } from '@/features/agent/composer/composer-choice-carousel'
import { choice } from '@/__tests__/__fixtures__/agent-choice'

const { answerChoiceFn } = vi.hoisted(() => ({ answerChoiceFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  answerChoice: (...args: unknown[]) => answerChoiceFn(...args),
}))

beforeEach(() => {
  answerChoiceFn.mockReset()
  answerChoiceFn.mockResolvedValue(undefined)
})

function ask(id: string, over: Partial<AskItem['choice']> = {}): AskItem {
  return {
    id,
    kind: 'ask',
    scope: 's',
    toolTarget: '',
    choice: choice({ id, ...over }),
  } as AskItem
}

const questionAsk = (id: string) =>
  ask(id, {
    kind: 'question',
    options: [],
    questions: [
      {
        id: 'q0',
        text: `Pick for ${id}`,
        multi: false,
        options: [
          { id: `${id}-a`, kind: 'answer', label: `A ${id}` },
          { id: `${id}-b`, kind: 'answer', label: `B ${id}` },
        ],
      },
    ],
  })

function mount(asks: AskItem[]) {
  const props = { wsId: 'w1', chatId: 'c1', providerLabel: 'Provider A', onOpenTerminal: vi.fn() }
  const view = render(<ComposerChoiceCarousel asks={asks} {...props} />)
  return {
    ...view,
    update: (next: AskItem[]) => view.rerender(<ComposerChoiceCarousel asks={next} {...props} />),
  }
}

const visible = () =>
  screen.getAllByTestId('agent-choice-prompt').filter((card) => !card.hasAttribute('hidden'))

describe('ComposerChoiceCarousel', () => {
  it('shows one ask at a time with an N of M stepper', () => {
    mount([ask('k1'), ask('k2'), ask('k3')])

    expect(visible()).toHaveLength(1)
    expect(visible()[0]).toHaveTextContent('1 of 3')
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Next request' }))
    expect(visible()[0].dataset.choiceId).toBe('k2')
    expect(visible()[0]).toHaveTextContent('2 of 3')
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Previous request' }))
    expect(visible()[0].dataset.choiceId).toBe('k1')
  })

  it('draws no stepper for a single ask', () => {
    mount([ask('k1')])

    expect(screen.queryByTestId('agent-choice-position')).toBeNull()
  })

  it('keeps the picks of an ask set aside', () => {
    mount([questionAsk('k1'), questionAsk('k2')])

    fireEvent.click(screen.getByRole('radio', { name: 'B k1' }))
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Next request' }))
    expect(visible()[0].dataset.choiceId).toBe('k2')
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Previous request' }))

    expect(screen.getByRole('radio', { name: 'B k1' })).toBeChecked()
  })

  it('Skip moves on without answering', () => {
    mount([questionAsk('k1'), questionAsk('k2')])

    fireEvent.click(screen.getByRole('button', { name: 'Skip' }))

    expect(visible()[0].dataset.choiceId).toBe('k2')
    expect(answerChoiceFn).not.toHaveBeenCalled()
  })

  it('answers only the ask on screen by key', async () => {
    mount([ask('k1'), ask('k2')])
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Next request' }))

    fireEvent.keyDown(visible()[0], { key: '1' })

    await waitFor(() =>
      expect(answerChoiceFn).toHaveBeenCalledWith('w1', 'c1', 'k2', { optionIds: ['allow'] }),
    )
    expect(answerChoiceFn).toHaveBeenCalledTimes(1)
  })

  it('stays on a real ask when the one on screen resolves', () => {
    const { update } = mount([ask('k1'), ask('k2')])
    fireEvent.click(within(visible()[0]).getByRole('button', { name: 'Next request' }))

    update([ask('k1')])

    expect(visible()).toHaveLength(1)
    expect(visible()[0].dataset.choiceId).toBe('k1')
    expect(screen.queryByTestId('agent-choice-position')).toBeNull()
  })
})

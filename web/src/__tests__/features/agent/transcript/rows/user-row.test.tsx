import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import * as commands from '@/features/agent/api/agent-chat-commands'
import * as feeds from '@/features/agent/stores/chat-feed'
import { retryPrompt, sendPrompt } from '@/features/agent/stores/chat-outbox'
import { chatStore } from '@/features/agent/stores/chat-store'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import { ApiError } from '@/lib/errors/api-error'
import * as errors from '@/lib/errors/report'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

// jsdom measures 0×0, and a zero-height viewport windows down to no rows; the
// opening gate keeps the list hidden, so controls are found by label.
const rect = { top: 0, left: 0, right: 768, bottom: 800, width: 768, height: 800, x: 0, y: 0 }
const originalRect = HTMLElement.prototype.getBoundingClientRect

let submit: MockInstance<typeof commands.submitAgentPrompt>
const actions = { retry: vi.fn(), edit: vi.fn(), cancel: vi.fn() }

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
  submit = vi
    .spyOn(commands, 'submitAgentPrompt')
    .mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
  vi.spyOn(feeds, 'attachFeed').mockReturnValue(() => {})
  vi.spyOn(errors, 'report').mockImplementation(() => {})
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalRect
  vi.restoreAllMocks()
  for (const fn of Object.values(actions)) fn.mockReset()
})

function draw() {
  render(
    <AgentTranscript
      chatId="c1"
      providers={[]}
      status="live"
      working={false}
      line={{ working: false }}
      onOpenTerminal={() => {}}
      promptActions={{ ...noRowActions, ...actions }}
    />,
  )
}

const rowOf = (requestId: string) => document.querySelector(`[data-row-id="p:${requestId}"]`)
const queuedId = () => chatStore.getState().outbox.c1?.[0]?.requestId ?? ''

describe('UserRow', () => {
  it('shows the prompt in the same render as Enter', () => {
    seedFeed('c1', [])
    draw()

    act(() => void sendPrompt('w1', 'c1', { text: 'right away', steer: false }))

    expect(screen.getByText('right away')).toBeInTheDocument()
    expect(rowOf(queuedId())).toHaveAttribute('data-state', 'sending')
  })

  it('keeps the same row element from queued to settled', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r0', 'earlier'), l.start('turn-0', 'r0')])
    draw()
    act(() => void sendPrompt('w1', 'c1', { text: 'next please', steer: false }))
    const requestId = queuedId()
    const row = rowOf(requestId)
    expect(row).toHaveAttribute('data-state', 'queued')

    await act(async () => seedFeed('c1', [l.end('turn-0')]))
    expect(rowOf(requestId)).toBe(row)
    expect(row).toHaveAttribute('data-state', 'sending')

    act(() => seedFeed('c1', [l.prompt(requestId, 'next please')]))
    expect(rowOf(requestId)).toBe(row)
    expect(row).toHaveAttribute('data-state', 'pending')

    act(() => seedFeed('c1', [l.start('turn-1', requestId), l.end('turn-1'), l.settle(requestId)]))
    expect(rowOf(requestId)).toBe(row)
    expect(row).not.toHaveAttribute('data-state')
    expect(screen.getAllByText('next please')).toHaveLength(1)
  })

  it('offers Retry and Edit when a send fails', async () => {
    seedFeed('c1', [])
    submit.mockRejectedValueOnce(new ApiError('model is unknown: invalid argument', 400))
    draw()
    act(() => void sendPrompt('w1', 'c1', { text: 'try me', steer: false }))
    const requestId = queuedId()
    await act(async () => {})

    expect(screen.getByText('model is unknown: invalid argument')).toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('Retry prompt'))
    expect(actions.retry).toHaveBeenCalledWith(requestId)
    fireEvent.click(screen.getByLabelText('Edit prompt'))
    expect(actions.edit).toHaveBeenCalledWith(requestId, 'try me')
  })

  it('a prompt the lost session refused says so and offers Retry', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'hello?'), l.settle('r1', false, 'session_lost')])
    draw()

    expect(screen.getByText(/session was lost\. start a new conversation/i)).toBeInTheDocument()
    expect(screen.queryByText(/may already have/i)).not.toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('Retry prompt'))
    expect(actions.retry).toHaveBeenCalledWith('r1')
  })

  it('a command the CLI ran itself says it ran in the terminal, with no Retry', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', '/context'), l.settle('r1', true, 'provider_builtin')])
    draw()

    expect(screen.getByText(/ran in the terminal/i)).toBeInTheDocument()
    expect(screen.queryByLabelText('Retry prompt')).not.toBeInTheDocument()
  })

  it('drops Retry from a refused row once its retry is on the way', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'hello?'), l.settle('r1', false, 'not_acknowledged')])
    submit.mockReturnValue(new Promise(() => {}))
    draw()
    expect(screen.getByLabelText('Retry prompt')).toBeInTheDocument()

    act(() => retryPrompt('w1', 'c1', 'r1', false))

    expect(rowOf('r1')).not.toBeNull()
    expect(screen.queryByLabelText('Retry prompt')).not.toBeInTheDocument()
  })
})

import { createElement, createRef } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage, AgentProvider, SlashCatalog } from '@/features/agent/api/agent-api'
import { promptQueueStorageKey } from '@/features/agent/lib/prompt-queue-persistence'
import { ApiError } from '@/lib/api'

const { listMessagesFn, submitPromptFn, slashCatalogFn } = vi.hoisted(() => ({
  listMessagesFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  listChatMessages: (...args: unknown[]) => listMessagesFn(...args),
  submitAgentPrompt: (...args: unknown[]) => submitPromptFn(...args),
  getSlashCatalog: (...args: unknown[]) => slashCatalogFn(...args),
}))

vi.mock('@/features/panes/lib/markdown', () => ({
  MarkdownPreview: ({ children }: { children: string }) => createElement('div', null, children),
}))

import {
  AgentChatView,
  type AgentChatViewHandle,
} from '@/features/agent/components/agent-chat-view'

const providers: AgentProvider[] = [
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
]

let initialMessages: AgentChatMessage[]
let incrementalMessages: AgentChatMessage[]
let olderMessages: AgentChatMessage[]

function page(items: AgentChatMessage[], hasMore = false) {
  return {
    cursor: items.at(-1)?.sequence ?? initialMessages.at(-1)?.sequence ?? 0,
    oldestCursor: items[0]?.sequence ?? initialMessages[0]?.sequence ?? 0,
    hasMore,
    items,
  }
}

function message(sequence: number, role: 'user' | 'assistant', text: string, providerId = 'codex') {
  return {
    sequence,
    role,
    providerId,
    text,
    at: `2026-08-16T00:00:${String(sequence).padStart(2, '0')}Z`,
  } satisfies AgentChatMessage
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const baseProps = () => ({
  wsId: 'w1',
  chatId: 'c1',
  providerId: 'codex',
  providers,
  working: false,
  turnRevision: 0,
  live: true,
  active: true,
  visible: true,
  onOpenTerminal: vi.fn(),
  onPromptSpawned: vi.fn(),
  onPromptDispatchStart: vi.fn(),
  onPromptDispatchSettled: vi.fn(),
  onRefreshChat: vi.fn().mockResolvedValue(true),
  onQueueCountChange: vi.fn(),
})

function setup(overrides: Partial<ReturnType<typeof baseProps>> = {}) {
  const props = { ...baseProps(), ...overrides }
  const rendered = render(<AgentChatView {...props} />)
  return {
    props,
    ...rendered,
    rerenderProps(next: Partial<typeof props>) {
      Object.assign(props, next)
      rendered.rerender(<AgentChatView {...props} />)
    },
  }
}

async function composer() {
  return screen.findByRole('textbox', { name: /message the agent/i })
}

async function enterPrompt(text: string) {
  const input = await composer()
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter', shiftKey: false })
}

beforeEach(() => {
  localStorage.clear()
  vi.useRealTimers()
  initialMessages = []
  incrementalMessages = []
  olderMessages = []
  listMessagesFn.mockReset()
  submitPromptFn.mockReset()
  slashCatalogFn.mockReset()
  listMessagesFn.mockImplementation(
    (_wsId: string, _chatId: string, options: { after?: number; before?: number }) => {
      if (options.before !== undefined) return Promise.resolve(page(olderMessages))
      if (options.after !== undefined) return Promise.resolve(page(incrementalMessages))
      return Promise.resolve(page(initialMessages))
    },
  )
  submitPromptFn.mockResolvedValue({ runnerId: 'r2', terminalSessionId: 'pty2' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  } satisfies SlashCatalog)
})

describe('AgentChatView message ledger', () => {
  it('renders complete hook-confirmed messages in sequence and attributes provider handoffs', async () => {
    initialMessages = [
      message(1, 'user', 'Question'),
      message(2, 'assistant', 'Codex answer'),
      message(3, 'assistant', 'Same provider'),
      message(4, 'assistant', 'Claude handoff', 'claude'),
    ]
    setup()

    expect(await screen.findByText('Question')).toBeInTheDocument()
    expect(screen.getByText('Codex answer')).toBeInTheDocument()
    expect(screen.getByText('Claude handoff')).toBeInTheDocument()
    expect(screen.getAllByText('Codex')).toHaveLength(1)
    expect(screen.getAllByText('Claude')).toHaveLength(1)
  })

  it('pages older messages upward and merges without duplicate sequences', async () => {
    initialMessages = [message(3, 'user', 'newer')]
    listMessagesFn.mockImplementation(
      (_w: string, _c: string, options: { after?: number; before?: number }) => {
        if (options.before !== undefined)
          return Promise.resolve({
            ...page([message(1, 'user', 'older'), message(3, 'user', 'dupe')]),
            hasMore: false,
          })
        if (options.after !== undefined) return Promise.resolve(page([]))
        return Promise.resolve({ ...page(initialMessages), hasMore: true })
      },
    )
    setup()
    fireEvent.click(await screen.findByRole('button', { name: /load earlier/i }))
    expect(await screen.findByText('older')).toBeInTheDocument()
    expect(screen.getAllByTestId(/^agent-message-\d+$/)).toHaveLength(2)
  })

  it('fetches after a batched start/stop pair even when working renders idle both times', async () => {
    const view = setup({ working: false, turnRevision: 0 })
    expect(await screen.findByText(/start the conversation/i)).toBeInTheDocument()

    // The workspace stream saw true then false in one React batch. `working`
    // therefore remains false, but the event revision advanced twice.
    incrementalMessages = [
      message(1, 'user', 'fast native turn'),
      message(2, 'assistant', 'fast answer'),
    ]
    view.rerenderProps({ working: false, turnRevision: 2 })

    expect(await screen.findByText('fast native turn')).toBeInTheDocument()
    expect(screen.getByText('fast answer')).toBeInTheDocument()
  })

  it('drains every incremental page when an idle hidden tab becomes visible', async () => {
    initialMessages = [message(1, 'assistant', 'before hiding')]
    const missed = Array.from({ length: 205 }, (_, index) =>
      message(index + 2, index % 2 === 0 ? 'user' : 'assistant', `missed ${index + 2}`),
    )
    listMessagesFn.mockImplementation(
      (_w: string, _c: string, options: { after?: number; limit?: number }) => {
        if (options.after !== undefined) {
          const limit = options.limit ?? 100
          const eligible = missed.filter((item) => item.sequence > (options.after ?? 0))
          return Promise.resolve(page(eligible.slice(0, limit), eligible.length > limit))
        }
        return Promise.resolve(page(initialMessages))
      },
    )
    const view = setup({ visible: false, working: false, turnRevision: 0 })
    expect(await screen.findByText('before hiding')).toBeInTheDocument()

    view.rerenderProps({ visible: true, working: false, turnRevision: 2 })

    expect(await screen.findByText('missed 206')).toBeInTheDocument()
    expect(screen.getAllByTestId(/^agent-message-\d+$/)).toHaveLength(206)
    expect(listMessagesFn.mock.calls.filter((call) => call[2]?.after !== undefined)).toHaveLength(3)
  })

  it('stops incremental catch-up when hasMore does not advance the cursor', async () => {
    initialMessages = [message(1, 'assistant', 'stable cursor')]
    listMessagesFn.mockImplementation((_w: string, _c: string, options: { after?: number }) =>
      options.after === undefined
        ? Promise.resolve(page(initialMessages))
        : Promise.resolve(page([message(1, 'assistant', 'stable cursor')], true)),
    )
    const view = setup({ visible: false })
    expect(await screen.findByText('stable cursor')).toBeInTheDocument()
    listMessagesFn.mockClear()

    view.rerenderProps({ visible: true, turnRevision: 1 })

    await waitFor(() => expect(listMessagesFn).toHaveBeenCalledTimes(1))
    await act(async () => Promise.resolve())
    expect(listMessagesFn).toHaveBeenCalledTimes(1)
  })
})

describe('AgentChatView durable FIFO', () => {
  it('queues while working, sends one at idle, and blocks the second until hook confirmation', async () => {
    const first = deferred<{ runnerId: string; terminalSessionId: string }>()
    submitPromptFn.mockReturnValueOnce(first.promise).mockResolvedValueOnce({
      runnerId: 'r3',
      terminalSessionId: 'pty3',
    })
    const view = setup({ working: true })

    await enterPrompt('first')
    await enterPrompt('second')
    expect(screen.getAllByTestId('queued-prompt')).toHaveLength(2)
    expect(submitPromptFn).not.toHaveBeenCalled()

    view.rerenderProps({ working: false })
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
    const firstId = submitPromptFn.mock.calls[0]?.[3]
    expect(submitPromptFn.mock.calls[0]?.slice(0, 3)).toEqual(['w1', 'c1', 'first'])

    await act(async () => first.resolve({ runnerId: 'r2', terminalSessionId: 'pty2' }))
    expect(screen.getByText(/waiting for provider confirmation/i)).toBeInTheDocument()
    expect(submitPromptFn).toHaveBeenCalledTimes(1)

    incrementalMessages = [message(1, 'user', 'first')]
    view.rerenderProps({ working: true })
    await waitFor(() => expect(screen.getAllByTestId('queued-prompt')).toHaveLength(1))
    expect(submitPromptFn).toHaveBeenCalledTimes(1)

    view.rerenderProps({ working: false })
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(2))
    expect(submitPromptFn.mock.calls[1]?.[2]).toBe('second')
    expect(firstId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
    )
  })

  it('does not dispatch queued work while Terminal is active or the pane is hidden', async () => {
    const view = setup({ working: true })
    await enterPrompt('wait for chat')
    view.rerenderProps({ working: false, active: false })
    await act(async () => Promise.resolve())
    expect(submitPromptFn).not.toHaveBeenCalled()

    view.rerenderProps({ active: true, visible: false })
    await act(async () => Promise.resolve())
    expect(submitPromptFn).not.toHaveBeenCalled()

    view.rerenderProps({ visible: true })
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
  })

  it('waits for a real idle edge after chat_busy before retrying with the same id', async () => {
    initialMessages = [message(7, 'assistant', 'existing ledger')]
    submitPromptFn
      .mockRejectedValueOnce(new ApiError('chat is working', 409, 'chat_busy'))
      .mockResolvedValueOnce({ runnerId: 'r2', terminalSessionId: 'pty2' })
    const refresh = vi.fn().mockResolvedValue(true)
    const view = setup({ onRefreshChat: refresh })
    expect(await screen.findByText('existing ledger')).toBeInTheDocument()
    await enterPrompt('race me')

    await waitFor(() => expect(screen.getByText(/became busy/i)).toBeInTheDocument())
    const firstId = submitPromptFn.mock.calls[0]?.[3]
    expect(submitPromptFn).toHaveBeenCalledTimes(1)

    // New provider output arriving before the idle retry must not move the
    // evidence baseline. A late user hook from the first call is still proof.
    incrementalMessages = [message(8, 'assistant', 'intervening output')]
    view.rerenderProps({ turnRevision: 1 })
    expect(await screen.findByText('intervening output')).toBeInTheDocument()
    view.rerenderProps({ working: true })
    view.rerenderProps({ working: false })
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(2))
    expect(submitPromptFn.mock.calls[1]?.[3]).toBe(firstId)
    const stored = JSON.parse(
      localStorage.getItem(promptQueueStorageKey('w1', 'c1')) ?? '{"items":[]}',
    ) as { items: Array<{ baselineSequence: number }> }
    expect(stored.items[0]?.baselineSequence).toBe(7)
  })

  it('retries after a batched busy-to-idle turn even when working renders false throughout', async () => {
    submitPromptFn
      .mockRejectedValueOnce(new ApiError('chat is working', 409, 'chat_busy'))
      .mockResolvedValueOnce({ runnerId: 'r2', terminalSessionId: 'pty2' })
    const refresh = vi.fn().mockResolvedValue(true)
    const view = setup({ working: false, turnRevision: 0, onRefreshChat: refresh })
    await enterPrompt('fast busy turn')

    await waitFor(() => expect(screen.getByText(/became busy/i)).toBeInTheDocument())
    const requestId = submitPromptFn.mock.calls[0]?.[3]
    expect(submitPromptFn).toHaveBeenCalledTimes(1)

    // Both authoritative lifecycle frames landed in one React batch. The
    // server-folded final value is idle and the monotonic revision proves a
    // later turn edge occurred, even though `working` never rendered true.
    view.rerenderProps({ working: false, turnRevision: 2 })

    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(2))
    expect(submitPromptFn.mock.calls[1]?.[3]).toBe(requestId)
  })

  it('rechecks a persisted chat_busy barrier after reload instead of wedging the FIFO', async () => {
    const clientRequestId = '11111111-1111-4111-8111-111111111111'
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId,
            text: 'survive reload',
            state: 'queued',
            createdAt: '2026-08-16T00:00:00Z',
            baselineSequence: 0,
            waitForIdleEpoch: 1,
          },
        ],
      }),
    )
    const refresh = vi.fn().mockResolvedValue(false)
    setup({ onRefreshChat: refresh })

    await waitFor(() => expect(refresh).toHaveBeenCalled())
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
    expect(submitPromptFn.mock.calls[0]?.slice(2)).toEqual(['survive reload', clientRequestId])
  })

  it('bulk-cancels only safely unsent rows and preserves every in-flight identity', async () => {
    const states = ['queued', 'failed', 'submitting', 'awaiting_turn', 'outcome_uncertain'] as const
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: states.map((state, index) => ({
          clientRequestId: `11111111-1111-4111-8111-11111111111${index}`,
          text: `${state} prompt`,
          state,
          createdAt: '2026-08-16T00:00:00Z',
          submittedAt:
            state === 'queued' || state === 'failed' ? undefined : '2026-08-16T00:00:01Z',
          baselineSequence: 0,
        })),
      }),
    )
    const ref = createRef<AgentChatViewHandle>()
    render(<AgentChatView {...baseProps()} working ref={ref} />)
    expect(await screen.findByText('queued prompt')).toBeInTheDocument()

    await act(async () => ref.current?.cancelUnsentPrompts())

    expect(screen.queryByText('queued prompt')).not.toBeInTheDocument()
    expect(screen.queryByText('failed prompt')).not.toBeInTheDocument()
    expect(screen.getByText('submitting prompt')).toBeInTheDocument()
    expect(screen.getByText('awaiting_turn prompt')).toBeInTheDocument()
    expect(screen.getByText('outcome_uncertain prompt')).toBeInTheDocument()
    const stored = JSON.parse(
      localStorage.getItem(promptQueueStorageKey('w1', 'c1')) ?? '{"items":[]}',
    ) as { items: Array<{ state: string }> }
    expect(stored.items.map((item) => item.state)).toEqual([
      'outcome_uncertain',
      'awaiting_turn',
      'outcome_uncertain',
    ])
  })

  it.each([
    ['request_outcome_uncertain', 'Delivery outcome unknown', 'outcome_uncertain'],
    ['request_id_conflict', 'different prompt text', 'failed'],
  ])('handles 409 code %s without automatic replay', async (code, copy, state) => {
    submitPromptFn.mockRejectedValueOnce(new ApiError('conflict', 409, code))
    setup()
    await enterPrompt('maybe delivered')
    await waitFor(() => expect(screen.getByText(new RegExp(copy, 'i'))).toBeInTheDocument())
    expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', state)
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
  })

  it('treats request_already_accepted as awaiting hook evidence, never resendable uncertainty', async () => {
    submitPromptFn.mockRejectedValueOnce(
      new ApiError('the original request was accepted', 409, 'request_already_accepted'),
    )
    setup()
    await enterPrompt('accepted once')
    expect(await screen.findByText(/waiting for provider confirmation/i)).toBeInTheDocument()
    expect(screen.queryByText(/delivery outcome unknown/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry same request/i })).not.toBeInTheDocument()
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
  })

  it.each([
    [new TypeError('Load failed'), 'transport rejection'],
    [new ApiError('unexpected server failure', 500), 'unexpected 5xx'],
  ])('treats %s (%s) as outcome-uncertain and never automatically replays', async (error) => {
    submitPromptFn.mockRejectedValueOnce(error)
    setup()
    await enterPrompt('possibly accepted')

    await waitFor(() =>
      expect(screen.getByTestId('queued-prompt')).toHaveAttribute(
        'data-state',
        'outcome_uncertain',
      ),
    )
    expect(screen.getByText(/delivery outcome unknown/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /edit queued prompt/i })).not.toBeInTheDocument()
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
    // Initial page plus a proactive evidence refresh after uncertainty.
    await waitFor(() => expect(listMessagesFn.mock.calls.length).toBeGreaterThan(1))
  })

  it('marks a vanished replacement uncertain, then clears it when the late user hook arrives', async () => {
    const view = setup()
    await enterPrompt('late hook')
    expect(await screen.findByText(/waiting for provider confirmation/i)).toBeInTheDocument()

    view.rerenderProps({ live: false })
    await waitFor(() =>
      expect(screen.getByTestId('queued-prompt')).toHaveAttribute(
        'data-state',
        'outcome_uncertain',
      ),
    )

    incrementalMessages = [message(1, 'user', 'late hook')]
    view.rerenderProps({ working: true })
    await waitFor(() => expect(screen.queryByTestId('queued-prompt')).not.toBeInTheDocument())
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
  })

  it('rehydrates pending prompts and converts interrupted submissions to explicit uncertainty', async () => {
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: 'dd068894-7cf0-4a2c-aa7c-c08531805bb0',
            text: 'survive reload',
            state: 'submitting',
            createdAt: '2026-08-16T00:00:00.000Z',
            baselineSequence: 0,
          },
        ],
      }),
    )
    setup()
    expect(await screen.findByText('survive reload')).toBeInTheDocument()
    expect(screen.getByText(/delivery outcome unknown/i)).toBeInTheDocument()
    expect(submitPromptFn).not.toHaveBeenCalled()
  })

  it('recovers persisted hook confirmation below the newest message page', async () => {
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: '23d00c6a-35ae-4d6c-a7c8-5e27983924f3',
            text: 'confirmation below latest page',
            state: 'awaiting_turn',
            createdAt: '2026-08-16T00:00:00Z',
            submittedAt: '2026-08-16T00:00:01Z',
            baselineSequence: 7,
          },
        ],
      }),
    )
    const newest = Array.from({ length: 100 }, (_, index) =>
      message(index + 108, 'assistant', `newer ${index + 108}`),
    )
    const recovery = [
      message(8, 'user', 'confirmation below latest page'),
      ...Array.from({ length: 99 }, (_, index) =>
        message(index + 9, 'assistant', `older ${index + 9}`),
      ),
    ]
    listMessagesFn.mockImplementation((_w: string, _c: string, options: { after?: number }) => {
      if (options.after === 7) return Promise.resolve(page(recovery, true))
      if (options.after !== undefined) return Promise.resolve(page(newest, false))
      return Promise.resolve(page(newest, true))
    })
    setup()

    await waitFor(() =>
      expect(listMessagesFn.mock.calls.some((call) => call[2]?.after === 7)).toBe(true),
    )
    expect(await screen.findByText('confirmation below latest page')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByTestId('queued-prompt')).not.toBeInTheDocument())
    expect(localStorage.getItem(promptQueueStorageKey('w1', 'c1'))).toBeNull()
    expect(submitPromptFn).not.toHaveBeenCalled()
  })

  it('reconciles a persisted awaiting prompt from hook evidence before considering dispatch', async () => {
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: 'dd068894-7cf0-4a2c-aa7c-c08531805bb0',
            text: 'confirmed during reload',
            state: 'awaiting_turn',
            createdAt: '2026-08-16T00:00:00.000Z',
            baselineSequence: 2,
          },
        ],
      }),
    )
    initialMessages = [message(3, 'user', 'confirmed during reload')]
    setup()

    expect(await screen.findByTestId('agent-message-3')).toHaveTextContent(
      'confirmed during reload',
    )
    await waitFor(() => expect(screen.queryByTestId('queued-prompt')).not.toBeInTheDocument())
    expect(localStorage.getItem(promptQueueStorageKey('w1', 'c1'))).toBeNull()
    expect(submitPromptFn).not.toHaveBeenCalled()
  })

  it('keeps Shift+Enter multiline content in the composer and sends it as one prompt', async () => {
    setup({ working: true })
    const input = await composer()
    fireEvent.change(input, { target: { value: 'line one\nline two' } })
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(submitPromptFn).not.toHaveBeenCalled()
    expect(input).toHaveValue('line one\nline two')
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(screen.getByTestId('queued-prompt')).toHaveTextContent(/line one\s+line two/)
  })

  it('offers a terminal detour when an accepted prompt may be waiting for trust input', async () => {
    const openTerminal = vi.fn()
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: 'dd068894-7cf0-4a2c-aa7c-c08531805bb0',
            text: 'waiting behind trust',
            state: 'awaiting_turn',
            createdAt: new Date(Date.now() - 7_000).toISOString(),
            submittedAt: new Date(Date.now() - 7_000).toISOString(),
            baselineSequence: 0,
          },
        ],
      }),
    )
    setup({ onOpenTerminal: openTerminal })

    expect(await screen.findByText(/waiting for a trust or permission answer/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))
    expect(openTerminal).toHaveBeenCalledTimes(1)
    expect(submitPromptFn).not.toHaveBeenCalled()
  })

  it('latches React submission off only for the permanent unsupported capability code', async () => {
    submitPromptFn.mockRejectedValueOnce(
      new ApiError('React submission unsupported', 422, 'prompt_submit_unsupported'),
    )
    setup()
    await enterPrompt('unsupported here')

    expect(await screen.findByText(/does not support React prompt submission/i)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /message the agent/i })).not.toBeInTheDocument()
  })

  it('keeps the composer available after the transient live_tui_required rejection', async () => {
    submitPromptFn.mockRejectedValueOnce(
      new ApiError('Open the native TUI first', 422, 'live_tui_required'),
    )
    setup()
    await enterPrompt('needs live tui')

    expect(await screen.findByText('Open the native TUI first')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()
    expect(screen.queryByText(/does not support React prompt submission/i)).not.toBeInTheDocument()
  })
})

describe('AgentChatView slash catalog', () => {
  it('does not submit a slash query as a model prompt while the deterministic probe is open', async () => {
    vi.useFakeTimers()
    const catalog = deferred<SlashCatalog>()
    slashCatalogFn.mockReturnValue(catalog.promise)
    setup()
    const input = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(input, { target: { value: '/' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(submitPromptFn).not.toHaveBeenCalled()
    expect(input).toHaveValue('/')
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(slashCatalogFn).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('debounces one live probe, filters locally, and inserts provider-mapped text without submitting', async () => {
    vi.useFakeTimers()
    slashCatalogFn.mockResolvedValue({
      providerId: 'codex',
      completeness: 'model_visible',
      warnings: [],
      items: [
        {
          id: 'one',
          kind: 'skill',
          label: 'review-code',
          description: 'Review a change',
          insertText: '$review-code ',
          source: 'builtin',
        },
        {
          id: 'two',
          kind: 'skill',
          label: 'write-tests',
          description: 'Add tests',
          insertText: '$write-tests ',
          source: 'builtin',
        },
      ],
    } satisfies SlashCatalog)
    setup()
    const input = screen.getByRole('textbox', { name: /message the agent/i })
    fireEvent.change(input, { target: { value: '/rev' } })
    expect(slashCatalogFn).not.toHaveBeenCalled()
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(slashCatalogFn).toHaveBeenCalledTimes(1)
    expect(screen.getByText('review-code')).toBeInTheDocument()
    expect(screen.queryByText('write-tests')).not.toBeInTheDocument()
    expect(screen.getByText(/model-visible skills/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('option', { name: /review-code/i }))
    expect(input).toHaveValue('$review-code ')
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('aborts and discards a stale catalog when the provider changes', async () => {
    vi.useFakeTimers()
    const catalog = deferred<SlashCatalog>()
    let signal: AbortSignal | undefined
    slashCatalogFn.mockImplementation((_w: string, _c: string, requestSignal: AbortSignal) => {
      signal = requestSignal
      return catalog.promise
    })
    const view = setup()
    fireEvent.change(screen.getByRole('textbox', { name: /message the agent/i }), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(signal?.aborted).toBe(false)
    view.rerenderProps({ providerId: 'claude' })
    expect(signal?.aborted).toBe(true)
    await act(async () =>
      catalog.resolve({
        providerId: 'codex',
        completeness: 'complete',
        warnings: [],
        items: [
          {
            id: 'stale',
            kind: 'skill',
            label: 'stale-skill',
            description: '',
            insertText: '$stale ',
            source: '',
          },
        ],
      }),
    )
    expect(screen.queryByText('stale-skill')).not.toBeInTheDocument()
    vi.useRealTimers()
  })

  it('discards a catalog response whose provider identity does not match the request', async () => {
    vi.useFakeTimers()
    slashCatalogFn.mockResolvedValue({
      providerId: 'claude',
      completeness: 'plugin_only',
      warnings: [],
      items: [
        {
          id: 'wrong-provider',
          kind: 'skill',
          label: 'wrong-provider-skill',
          description: '',
          insertText: '/wrong-provider-skill ',
          source: '',
        },
      ],
    } satisfies SlashCatalog)
    setup({ providerId: 'codex' })
    fireEvent.change(screen.getByRole('textbox', { name: /message the agent/i }), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))

    expect(screen.queryByText('wrong-provider-skill')).not.toBeInTheDocument()
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('offers the native terminal when the provider has no deterministic catalog', async () => {
    vi.useFakeTimers()
    const openTerminal = vi.fn()
    slashCatalogFn.mockRejectedValue(new ApiError('catalog unsupported', 422))
    setup({ onOpenTerminal: openTerminal })
    fireEvent.change(screen.getByRole('textbox', { name: /message the agent/i }), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(screen.getByText(/no react skill catalog/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))
    expect(openTerminal).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })
})

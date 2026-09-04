import { createElement, createRef } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage, AgentProvider, SlashCatalog } from '@/features/agent/api/agent-api'
import { promptQueueStorageKey } from '@/features/agent/lib/prompt-queue-persistence'
import { ApiError } from '@/lib/api'
import { __resetPerfForTests } from '@/lib/perf/instrumentation'
import { ESTIMATED_ROW_HEIGHT } from '@/features/agent/transcript/agent-transcript'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

const { listMessagesFn, submitPromptFn, slashCatalogFn, setSelectionFn, stopChatFn } = vi.hoisted(
  () => ({
    listMessagesFn: vi.fn(),
    submitPromptFn: vi.fn(),
    slashCatalogFn: vi.fn(),
    setSelectionFn: vi.fn(),
    stopChatFn: vi.fn(),
  }),
)

vi.mock('@/features/agent/api/agent-api', () => ({
  listChatMessages: (...args: unknown[]) => listMessagesFn(...args),
  submitAgentPrompt: (...args: unknown[]) => submitPromptFn(...args),
  getSlashCatalog: (...args: unknown[]) => slashCatalogFn(...args),
  setChatSelection: (...args: unknown[]) => setSelectionFn(...args),
  listChatActivity: (...args: unknown[]) => activityFn(...args),
  stopChat: (...args: unknown[]) => stopChatFn(...args),
}))

// The prompt box is a Plate editor, and **jsdom never delivers a keydown to a
// Slate editable** — measured: window- and document-capture see the event, the
// editable's own listeners never fire. Neither `PlateContent onKeyDown` nor a
// plugin handler runs. So a test that typed into the real editor here would not
// be testing the queue, it would be testing nothing and passing.
//
// These suites are about the QUEUE, the catalog and the ledger. The editor gets
// a stand-in with the same contract — text in, markdown out, keys through — and
// the editor's own behaviour is verified live and in its own suite.
vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: ({
    initialValue,
    placeholder,
    ariaLabel,
    onChange,
    onKeyDown,
    expanded,
    controls,
  }: {
    initialValue: string
    placeholder: string
    ariaLabel: string
    onChange: (value: string) => void
    onKeyDown: (
      event: unknown,
      readMarkdown: () => string,
      caret: { atStart: boolean; atEnd: boolean },
    ) => void
    expanded?: boolean
    controls?: string
  }) =>
    createElement('textarea', {
      'aria-label': ariaLabel,
      'aria-expanded': expanded,
      'aria-controls': controls,
      placeholder,
      defaultValue: initialValue,
      onChange: (event: { target: { value: string } }) => onChange(event.target.value),
      // Second argument included deliberately: the real editor hands the key
      // handler the BOX's text, and a mock that omitted it would let a submit
      // path that reads stale state keep passing. Third argument is a stand-in
      // for the real editor's own caret-edge probe — jsdom does not execute a
      // textarea's native arrow-key caret movement the way a real browser does,
      // so this suite (about the queue/catalog/ledger, per the note above, not
      // caret geometry) always reports "at an edge" rather than trying to track
      // a position jsdom never actually moves.
      onKeyDown: (event: { currentTarget: { value: string } }) =>
        onKeyDown(event, () => event.currentTarget.value, { atStart: true, atEnd: true }),
    }),
}))

// BOTH renderers, for the one reason the interactive one was already stubbed:
// this suite is about the queue, the catalog and the ledger, not about which
// markdown engine draws a settled turn (message-row.test.tsx owns that split).
// A settled row renders through MarkdownMessageStatic, so leaving that one real
// meant these ledger fixtures — two of which are 200+ messages long — each built
// a full Plate document per row, for text the assertions only ever read back
// verbatim.
vi.mock('@/features/agent/transcript/plate/markdown-message', () => ({
  MarkdownMessage: ({ children }: { children: string }) => createElement('div', null, children),
}))

vi.mock('@/features/agent/transcript/plate/markdown-message-static', () => ({
  MarkdownMessageStatic: ({ children }: { children: string }) =>
    createElement('div', null, children),
}))

import { AgentChatView, type AgentChatViewHandle } from '@/features/agent/chat/agent-chat-view'

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
const activityFn = vi.fn()
const emptyActivity = { toolCalls: [], subagents: [], interruptions: [], choices: [] }
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
    turnId: `turn-${sequence}`,
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
  // Declared so `setup`/`rerenderProps` accept it: the daemon only ever sends
  // these for a delivery that produced no turn, so the default is none.
  settledPrompts: undefined as string[] | undefined,
  streamingMessages: undefined as { id: string; text: string }[] | undefined,
  // No sticky selection: these fixtures' providers declare no catalogue, so the
  // picker renders nothing at all here (see agent-model-picker.test.tsx).
  model: '',
  effort: '',
  onSelectionChange: vi.fn(),
  // The surface controls moved into the chat's own provider bar, so the pane
  // hands their state down rather than drawing them itself.
  presentation: 'chat' as const,
  splitEnabled: false,
  onSelectPresentation: vi.fn(),
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

/**
 * Wherever this chat is currently asking to be written into.
 *
 * A blank chat is the DOCUMENT — 16px, its own measure, no bar under it — and it
 * becomes the pill the instant there is a conversation. Both are the same
 * gesture to a person, so the harness types into whichever is on screen rather
 * than making every test declare which surface it expects to be looking at.
 */
async function composer() {
  const find = () =>
    screen.queryByRole('textbox', { name: /message the agent|describe the change/i })
  // The surface is not chosen until the ledger's first page lands, so the box does
  // not exist on the first tick. Flushed with `act` rather than awaited with
  // `findBy`, because half these tests run on fake timers and a poll-based wait
  // never advances under them.
  for (let i = 0; i < 3 && !find(); i++) await act(async () => {})
  return find() ?? (await screen.findByRole('textbox', { name: /message the agent/i }))
}

async function enterPrompt(text: string) {
  const input = await composer()
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter', shiftKey: false })
}

// The transcript's historical rows are windowed (`@tanstack/react-virtual`),
// and jsdom has no layout engine, which breaks the virtualiser two ways:
//
//  - The scroll container measures 0px tall, and a virtualiser told its
//    viewport is zero pixels windows down to NOTHING — `calculateRange` bails
//    on `outerSize === 0` before overscan is ever applied, so not one message
//    mounts.
//  - Every ROW measures 0px too, so every row's `start` is 0 and the range's
//    binary search — which returns on the first exact hit — lands in the
//    MIDDLE of the list, silently unmounting the oldest half.
//
// These are ledger suites: they assert on the messages the ledger merged, so
// the window has to cover the whole conversation, not a viewport's worth. A
// nominal 1px per row keeps the starts strictly increasing (so the search finds
// index 0), and an absurdly tall viewport keeps the window's far end past the
// newest message. Rows shown/hidden by the WINDOW are agent-transcript.test.tsx's
// subject, not this file's.
//
// Everything else (the dock's own measurement, the composer handle's drag
// geometry) keeps jsdom's zeros, so nothing but the window changes here.
//
// The row height deliberately MATCHES the transcript's own `estimateSize`
// (imported above, not re-hardcoded), so the first measurement of every row
// reports no change and the virtualiser never notifies — one render pass for
// a 200-message ledger instead of two.
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

function fakeRect(width: number, height: number): DOMRect {
  const rect = { top: 0, left: 0, right: width, bottom: height, width, height, x: 0, y: 0 }
  return { ...rect, toJSON: () => rect } as DOMRect
}

beforeEach(() => {
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect(this: HTMLElement) {
    if (this.dataset.testid === 'agent-message-list') return fakeRect(768, 1_000_000)
    if (this.hasAttribute('data-index')) return fakeRect(768, ESTIMATED_ROW_HEIGHT)
    return originalGetBoundingClientRect.call(this)
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

beforeEach(() => {
  localStorage.clear()
  vi.useRealTimers()
  activityFn.mockReset()
  activityFn.mockResolvedValue(emptyActivity)
  initialMessages = []
  incrementalMessages = []
  olderMessages = []
  listMessagesFn.mockReset()
  submitPromptFn.mockReset()
  slashCatalogFn.mockReset()
  setSelectionFn.mockReset()
  setSelectionFn.mockResolvedValue(undefined)
  stopChatFn.mockReset()
  stopChatFn.mockResolvedValue(undefined)
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
  it('renders complete hook-confirmed messages in sequence, across a provider handoff', async () => {
    initialMessages = [
      message(1, 'user', 'Question'),
      message(2, 'assistant', 'Codex answer'),
      message(3, 'assistant', 'Same provider'),
      message(4, 'assistant', 'Claude handoff', 'claude'),
    ]
    setup()

    expect(await screen.findByText('Question')).toBeInTheDocument()
    expect(screen.getByText('Codex answer')).toBeInTheDocument()
    expect(screen.getByText('Same provider')).toBeInTheDocument()
    expect(screen.getByText('Claude handoff')).toBeInTheDocument()
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

  // The user's own reproduction: "Claude might say something, then call a tool,
  // and then say something else, and the message would appear ONLY when it
  // finished working."
  //
  // The daemon half is proven live (TestRegression_AMessageSaidMidTurnIsVISIBLE-
  // BeforeTheTurnEnds: the mid-turn message is readable from ReadMessages ~7s in,
  // with the chat still working). So what is left to pin is the pane: while
  // working stays TRUE the whole time — no turn boundary, no lifecycle frame, no
  // re-render from a prop change — the poll alone has to bring the new message in
  // and render it. Everything else in this file drives a working edge, which is
  // exactly the case that was never the bug.
  it('renders a message that arrives mid-turn, on the poll alone, with working never changing', async () => {
    initialMessages = [message(1, 'user', 'Do the thing')]
    incrementalMessages = []
    setup({ working: true })

    expect(await screen.findByText('Do the thing')).toBeInTheDocument()
    expect(screen.queryByText('ALPHA')).not.toBeInTheDocument()

    // The agent says its first message and reaches for a slow tool. Nothing about
    // the chat's working state changes — it is mid-turn throughout.
    incrementalMessages = [message(2, 'assistant', 'ALPHA')]

    await waitFor(
      () => {
        expect(screen.getByText('ALPHA')).toBeInTheDocument()
      },
      { timeout: 4_000 },
    )
  })

  it('fetches after a batched start/stop pair even when working renders idle both times', async () => {
    const view = setup({ working: false, turnRevision: 0 })
    expect(await screen.findByTestId('agent-empty-document')).toBeInTheDocument()

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
    expect(screen.getAllByTestId('queued-prompt')[0]).toHaveAttribute('data-state', 'awaiting_turn')
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
    ['request_outcome_uncertain', 'outcome_uncertain'],
    ['request_id_conflict', 'failed'],
  ])('handles 409 code %s without automatic replay', async (code, state) => {
    submitPromptFn.mockRejectedValueOnce(new ApiError('conflict', 409, code))
    setup()
    await enterPrompt('maybe delivered')
    await waitFor(() =>
      expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', state),
    )
    // The prompt's own words survive the failure — a retry must never be a retype.
    expect(screen.getByTestId('queued-prompt')).toHaveTextContent('maybe delivered')
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
  })

  it('treats request_already_accepted as awaiting hook evidence, never resendable uncertainty', async () => {
    submitPromptFn.mockRejectedValueOnce(
      new ApiError('the original request was accepted', 409, 'request_already_accepted'),
    )
    setup()
    await enterPrompt('accepted once')
    await waitFor(() =>
      expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', 'awaiting_turn'),
    )
    expect(screen.getByTestId('queued-prompt')).not.toHaveAttribute(
      'data-state',
      'outcome_uncertain',
    )
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
    expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', 'outcome_uncertain')
    expect(screen.queryByRole('button', { name: /edit queued prompt/i })).not.toBeInTheDocument()
    expect(submitPromptFn).toHaveBeenCalledTimes(1)
    // Initial page plus a proactive evidence refresh after uncertainty.
    await waitFor(() => expect(listMessagesFn.mock.calls.length).toBeGreaterThan(1))
  })

  it('marks a vanished replacement uncertain, then clears it when the late user hook arrives', async () => {
    const view = setup()
    await enterPrompt('late hook')
    await waitFor(() =>
      expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', 'awaiting_turn'),
    )

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
    expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', 'outcome_uncertain')
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

  // THE WEDGE. A provider's own built-in — /compact, measured against claude
  // 2.1.236 — is handled inside the CLI and produces no user message in the
  // ledger, so the only evidence this queue normally waits for never arrives. The
  // head sat in awaiting_turn forever and the composer refused every later prompt.
  it('releases a pending prompt the daemon reports as settled without a turn', async () => {
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: 'dd068894-7cf0-4a2c-aa7c-c08531805bb0',
            text: '/compact',
            state: 'awaiting_turn',
            createdAt: '2026-08-16T00:00:00.000Z',
            baselineSequence: 2,
          },
        ],
      }),
    )
    const view = setup()
    expect(await screen.findByTestId('queued-prompt')).toHaveTextContent('/compact')

    view.rerenderProps({ settledPrompts: ['dd068894-7cf0-4a2c-aa7c-c08531805bb0'] })

    await waitFor(() => expect(screen.queryByTestId('queued-prompt')).not.toBeInTheDocument())
    expect(submitPromptFn).not.toHaveBeenCalled()
  })

  // A settled id for some OTHER prompt must not take this one down with it.
  it('leaves a pending prompt alone when a different delivery settles', async () => {
    localStorage.setItem(
      promptQueueStorageKey('w1', 'c1'),
      JSON.stringify({
        version: 1,
        items: [
          {
            clientRequestId: 'dd068894-7cf0-4a2c-aa7c-c08531805bb0',
            text: '/compact',
            state: 'awaiting_turn',
            createdAt: '2026-08-16T00:00:00.000Z',
            baselineSequence: 2,
          },
        ],
      }),
    )
    const view = setup()
    expect(await screen.findByTestId('queued-prompt')).toHaveTextContent('/compact')

    view.rerenderProps({ settledPrompts: ['11111111-2222-3333-4444-555555555555'] })

    expect(screen.getByTestId('queued-prompt')).toHaveTextContent('/compact')
  })

  it('keeps Shift+Enter multiline content in the composer and sends it as one prompt', async () => {
    initialMessages = [message(1, 'assistant', 'earlier turn')]
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

    expect(await screen.findByLabelText('Open Terminal')).toBeInTheDocument()
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

    expect(await screen.findByText(/cannot accept a prompt typed here/i)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /message the agent/i })).not.toBeInTheDocument()
  })

  it('keeps the composer available after the transient live_tui_required rejection', async () => {
    submitPromptFn.mockRejectedValueOnce(
      new ApiError('Open the native TUI first', 422, 'live_tui_required'),
    )
    setup()
    await enterPrompt('needs live tui')

    expect(await screen.findByText('Open the native TUI first')).toBeInTheDocument()
    expect(await composer()).toBeInTheDocument()
    expect(screen.queryByText(/cannot accept a prompt typed here/i)).not.toBeInTheDocument()
  })
})

describe('AgentChatView composer controls', () => {
  // REGRESSION: the send button's onClick handed the click's own MouseEvent
  // straight to `enqueueDraft`, whose optional `text` parameter then read
  // that event instead of `undefined` — `prompts.enqueue` calls `.trim()` on
  // it and throws. Enter never went through this path, so only the button
  // was broken; a test that only pressed Enter would never have caught it.
  it('sends when the send button is clicked, not only on Enter', async () => {
    initialMessages = [message(1, 'assistant', 'hi')]
    setup()
    const input = await composer()
    fireEvent.change(input, { target: { value: 'click to send' } })

    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))

    await waitFor(() =>
      expect(submitPromptFn).toHaveBeenCalledWith('w1', 'c1', 'click to send', expect.any(String)),
    )
  })

  // The same click-handler bug existed on the blank chat's own send button
  // (AgentEmptyDocument's `onSubmit`), fixed identically.
  it('sends from a blank chat when its own send button is clicked', async () => {
    setup()
    const input = await composer()
    fireEvent.change(input, { target: { value: 'first ever message' } })

    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))

    await waitFor(() =>
      expect(submitPromptFn).toHaveBeenCalledWith(
        'w1',
        'c1',
        'first ever message',
        expect.any(String),
      ),
    )
  })

  // Shell-style recall through the chat's OWN sent turns, newest first —
  // never past what is actually loaded, and back to the live draft at the top.
  it("recalls the chat's own sent turns with ArrowUp/ArrowDown", async () => {
    initialMessages = [
      message(1, 'user', 'first message'),
      message(2, 'assistant', 'reply one'),
      message(3, 'user', 'second message'),
      message(4, 'assistant', 'reply two'),
    ]
    setup()
    expect(await screen.findByText('reply two')).toBeInTheDocument()

    fireEvent.keyDown(await composer(), { key: 'ArrowUp' })
    expect(await composer()).toHaveValue('second message')

    fireEvent.keyDown(await composer(), { key: 'ArrowUp' })
    expect(await composer()).toHaveValue('first message')

    // Nothing older than the oldest loaded turn — stays put.
    fireEvent.keyDown(await composer(), { key: 'ArrowUp' })
    expect(await composer()).toHaveValue('first message')

    fireEvent.keyDown(await composer(), { key: 'ArrowDown' })
    expect(await composer()).toHaveValue('second message')

    fireEvent.keyDown(await composer(), { key: 'ArrowDown' })
    // Back past the newest turn: the live draft, empty since nothing was typed.
    expect(await composer()).toHaveValue('')
  })

  // Enter reaches enqueueDraft even on an empty box (the send BUTTON is
  // disabled for empty, but Enter bypasses it) — nothing sent is correct, but
  // surfacing an alert for it is friction over nothing gone wrong.
  it('says nothing when Enter is pressed on an empty box — there is nothing to warn about', async () => {
    setup()
    fireEvent.keyDown(await composer(), { key: 'Enter', shiftKey: false })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(submitPromptFn).not.toHaveBeenCalled()
  })
})

describe('AgentChatView type-to-focus', () => {
  beforeEach(() => {
    setActiveWorkspaceId('w1')
  })

  const type = (...keys: string[]) => {
    for (const key of keys) fireEvent.keyDown(window, { key })
  }

  it('redirects into the composer once a fourth key is typed while nothing is focused', async () => {
    setup()
    await composer()
    type('h', 'e', 'l', 'l')
    expect(await composer()).toHaveValue('hell')
  })

  it('does not redirect on three keys or fewer', async () => {
    setup()
    await composer()
    type('h', 'e', 'l')
    expect(await composer()).toHaveValue('')
  })

  it('does not redirect when a different workspace is active', async () => {
    setActiveWorkspaceId('other-ws')
    setup()
    await composer()
    type('h', 'e', 'l', 'l')
    expect(await composer()).toHaveValue('')
  })

  it('does not redirect when the composer already has focus', async () => {
    setup()
    const input = await composer()
    act(() => input.focus())
    for (const key of ['h', 'e', 'l', 'l']) fireEvent.keyDown(input, { key })
    expect(await composer()).toHaveValue('')
  })

  it('does not redirect while the pane is inactive or hidden', async () => {
    const view = setup({ active: false })
    await composer()
    type('h', 'e', 'l', 'l')
    expect(await composer()).toHaveValue('')

    view.rerenderProps({ active: true, visible: false })
    type('w', 'o', 'r', 'd')
    expect(await composer()).toHaveValue('')
  })

  it('ignores a modifier-held key and leaves the buffer for the plain keys after it', async () => {
    setup()
    await composer()
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    type('h', 'e', 'l', 'l')
    expect(await composer()).toHaveValue('hell')
  })
})

describe('AgentChatView streaming', () => {
  // These are about the PILL: its slash picker, its multiline behaviour, the
  // bubble that streams above it. None of them exist on a blank chat, which is
  // the document surface — so each of these starts from a chat that has already
  // been spoken in.
  beforeEach(() => {
    initialMessages = [message(1, 'assistant', 'earlier turn')]
  })

  // The agent is mid-sentence. Its text is not in the ledger yet — a message that
  // is still growing is a view, not a record — so it arrives on the live feed and
  // must be rendered anyway. Without this the chat sits blank while the agent
  // visibly types in the terminal beside it.
  it('renders the message the agent is still saying', async () => {
    const view = setup()
    await screen.findByTestId('agent-message-list')

    view.rerenderProps({ streamingMessages: [{ id: 'm1', text: 'half a sen' }] })

    expect(await screen.findByText('half a sen')).toBeInTheDocument()
  })

  // Frames carry the text SO FAR, not the increment, so a client that missed one
  // is correct again on the next.
  it('replaces the partial text rather than appending to it', async () => {
    const view = setup()
    await screen.findByTestId('agent-message-list')

    view.rerenderProps({ streamingMessages: [{ id: 'm1', text: 'half a sen' }] })
    await screen.findByText('half a sen')
    view.rerenderProps({ streamingMessages: [{ id: 'm1', text: 'half a sentence' }] })

    expect(await screen.findByText('half a sentence')).toBeInTheDocument()
    expect(screen.queryByText('half a sen')).not.toBeInTheDocument()
  })

  // The live frame and the message poll arrive from different places, so for a
  // moment both hold the same sentence. Rendering both would show it twice.
  it('drops the partial once the ledger has the finished message', async () => {
    // turnId follows the real backend convention (assistantTurnID: "msg-"+the
    // streamed message's own id) — suppression is matched by id, not text, so
    // the fixture has to carry the id the streaming bubble below will match.
    initialMessages = [{ ...message(3, 'assistant', 'the whole sentence'), turnId: 'msg-m1' }]
    const view = setup()
    expect(await screen.findByTestId('agent-message-3')).toBeInTheDocument()

    view.rerenderProps({ streamingMessages: [{ id: 'm1', text: 'the whole sentence' }] })

    await waitFor(() => expect(screen.getAllByText('the whole sentence')).toHaveLength(1))
  })

  // Regression: Codex can split one turn's reply across more than one open
  // message item. The old model stored a single {id, text} slot per chat, so
  // a second item's first delta silently overwrote the first item's
  // still-growing text — it vanished from the live view (though it was safe
  // in the ledger all along), then reappeared once the real record landed.
  // This is the case that model can't represent at all: two DIFFERENT ids,
  // both still open, at once.
  it('renders more than one still-open message item at once, without either clobbering the other', async () => {
    const view = setup()
    await screen.findByTestId('agent-message-list')

    view.rerenderProps({
      streamingMessages: [
        { id: 'm1', text: 'first paragraph, still growing' },
        { id: 'm2', text: 'second item' },
      ],
    })

    expect(await screen.findByText('first paragraph, still growing')).toBeInTheDocument()
    expect(await screen.findByText('second item')).toBeInTheDocument()
  })

  it('only drops the specific item the ledger has confirmed, leaving the other streaming item visible', async () => {
    // See the "drops the partial" test above: turnId must match "msg-"+the
    // streamed id (m1 here) for the suppression this test is exercising.
    initialMessages = [{ ...message(3, 'assistant', 'first paragraph, done'), turnId: 'msg-m1' }]
    const view = setup()
    expect(await screen.findByTestId('agent-message-3')).toBeInTheDocument()

    view.rerenderProps({
      streamingMessages: [
        { id: 'm1', text: 'first paragraph, done' },
        { id: 'm2', text: 'second item, still growing' },
      ],
    })

    await waitFor(() => expect(screen.getAllByText('first paragraph, done')).toHaveLength(1))
    expect(screen.getByText('second item, still growing')).toBeInTheDocument()
  })
})

describe('AgentChatView slash catalog', () => {
  // These are about the PILL: its slash picker, its multiline behaviour, the
  // bubble that streams above it. None of them exist on a blank chat, which is
  // the document surface — so each of these starts from a chat that has already
  // been spoken in.
  beforeEach(() => {
    initialMessages = [message(1, 'assistant', 'earlier turn')]
  })

  // The picker owns Enter only while it has something to accept. It used to own
  // the key for as long as it was OPEN, which made every provider built-in
  // unsendable: no probe reports /compact, /clear, /model or /context, so the
  // picker sat there matching nothing while the composer said "Enter to send"
  // under a key that did nothing at all.
  it('sends a slash command the catalog cannot match, rather than swallowing Enter', async () => {
    vi.useFakeTimers()
    slashCatalogFn.mockResolvedValue({
      providerId: 'claude',
      completeness: 'plugin_only',
      warnings: [],
      items: [],
    })
    setup()
    const input = await composer()
    fireEvent.change(input, { target: { value: '/compact' } })
    // One presaved on mount, one silent refresh from opening the picker.
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(slashCatalogFn).toHaveBeenCalledTimes(2)

    fireEvent.keyDown(input, { key: 'Enter' })

    await act(async () => vi.advanceTimersByTimeAsync(0))
    expect(submitPromptFn).toHaveBeenCalledWith('w1', 'c1', '/compact', expect.any(String))
    vi.useRealTimers()
  })

  it('accepts the highlighted completion with Tab, which never sends', async () => {
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
      ],
    })
    setup()
    const input = await composer()
    fireEvent.change(input, { target: { value: '/rev' } })
    await act(async () => vi.advanceTimersByTimeAsync(150))

    fireEvent.keyDown(input, { key: 'Tab' })

    // RE-QUERIED, not the handle from before the insert. Text pushed in from
    // outside arrives by remounting the box (see `draftSeed`), so the element
    // that held the `/rev` the picker matched is detached by now.
    expect(await composer()).toHaveValue('$review-code ')
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  // Same rule, the other side of it: a probe that has not answered yet has
  // nothing to accept either, and blocking Enter behind it would make the user
  // wait out a ten-second provider timeout to send four characters.
  it('sends while the deterministic probe is still running', async () => {
    vi.useFakeTimers()
    const catalog = deferred<SlashCatalog>()
    slashCatalogFn.mockReturnValue(catalog.promise)
    setup()
    const input = await composer()
    fireEvent.change(input, { target: { value: '/clear' } })
    // One presaved on mount, one silent refresh from opening the picker —
    // neither has answered yet, and Enter must not wait for either.
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(slashCatalogFn).toHaveBeenCalledTimes(2)

    fireEvent.keyDown(input, { key: 'Enter' })

    await act(async () => vi.advanceTimersByTimeAsync(0))
    expect(submitPromptFn).toHaveBeenCalledWith('w1', 'c1', '/clear', expect.any(String))
    vi.useRealTimers()
  })

  it('shows the presaved catalog instantly on open, then refreshes it once, silently', async () => {
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
    // Presaved on chat initiation, well before the first `/`.
    await act(async () => vi.advanceTimersByTimeAsync(0))
    expect(slashCatalogFn).toHaveBeenCalledTimes(1)

    const input = await composer()
    fireEvent.change(input, { target: { value: '/rev' } })
    // Shows the cached catalog on the very same tick — no spinner, no wait.
    expect(screen.getByText('$review-code')).toBeInTheDocument()
    expect(screen.queryByText('write-tests')).not.toBeInTheDocument()
    expect(slashCatalogFn).toHaveBeenCalledTimes(1)

    // Opening it still asks again in the background, debounced, once.
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(slashCatalogFn).toHaveBeenCalledTimes(2)

    fireEvent.click(screen.getByRole('option', { name: /review-code/i }))
    // RE-QUERIED, not the handle from before the insert. Text pushed in from
    // outside arrives by remounting the box (see `draftSeed`), so the element
    // that held the `/rev` the picker matched is detached by now.
    expect(await composer()).toHaveValue('$review-code ')
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('aborts and discards a stale catalog when the provider changes', async () => {
    vi.useFakeTimers()
    const catalog = deferred<SlashCatalog>()
    const signals: AbortSignal[] = []
    slashCatalogFn.mockImplementation((_w: string, _c: string, requestSignal: AbortSignal) => {
      signals.push(requestSignal)
      return catalog.promise
    })
    const view = setup()
    // The chat's own presave already fired one probe on mount.
    expect(signals).toHaveLength(1)

    fireEvent.change(await composer(), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(signals).toHaveLength(2)
    const openSignal = signals[1]
    expect(openSignal.aborted).toBe(false)

    view.rerenderProps({ providerId: 'claude' })
    expect(openSignal.aborted).toBe(true)

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
    fireEvent.change(await composer(), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))

    expect(screen.queryByText('wrong-provider-skill')).not.toBeInTheDocument()
    expect(submitPromptFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('says a provider has no catalog without inventing chrome to say it in', async () => {
    vi.useFakeTimers()
    const onSelectPresentation = vi.fn()
    slashCatalogFn.mockRejectedValue(new ApiError('catalog unsupported', 422))
    setup({ onSelectPresentation })
    fireEvent.change(await composer(), {
      target: { value: '/' },
    })
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(screen.getByText(/no skill list from this provider/i)).toBeInTheDocument()
    // The way to the provider's own view is the switcher under the composer, not
    // a button the picker grows for this one case.
    fireEvent.click(screen.getByRole('tab', { name: /provider|terminal/i }))
    expect(onSelectPresentation).toHaveBeenCalledWith('terminal')
    vi.useRealTimers()
  })
})

describe('AgentChatView model + effort selection', () => {
  // The catalogue-declaring provider. The default fixtures deliberately declare
  // none, which is why every other test in this file sees no picker at all.
  const selectable: AgentProvider[] = [
    {
      ...providers[0],
      modelSelect: true,
      effortSelect: true,
      models: ['gpt-5.6-sol', 'gpt-5.6-luna'],
      efforts: {
        '': ['low', 'medium', 'high'],
        'gpt-5.6-sol': ['low', 'medium', 'high', 'max', 'ultra'],
        'gpt-5.6-luna': ['low', 'medium', 'high', 'max'],
      },
    },
    providers[1],
  ]

  it('shows no picker at all for a provider that declares no catalogue', async () => {
    setup()
    await composer()
    expect(screen.queryByTestId('agent-model-picker')).toBeNull()
  })

  it('puts the picker by the composer when the provider declares one', async () => {
    setup({ providers: selectable })
    await composer()
    expect(screen.getByTestId('agent-model-picker')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Model:/ })).toHaveTextContent('Default model')
  })

  it('writes a picked model and hands the accepted pair back to the chat owner', async () => {
    const onSelectionChange = vi.fn()
    setup({ providers: selectable, model: 'gpt-5.6-sol', effort: 'ultra', onSelectionChange })
    await composer()

    fireEvent.click(screen.getByRole('button', { name: /^Model:/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'gpt-5.6-luna' }))

    // `ultra` is not a gpt-5.6-luna level, so it is cleared in the same write.
    await waitFor(() => expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.6-luna', ''))
    expect(onSelectionChange).toHaveBeenCalledWith('gpt-5.6-luna', '')
  })

  // The reported effort used to render here; it is gone from the transcript
  // entirely now, replaced everywhere by the turnbar (provider icon + copy).
  it('shows turn actions on an assistant reply, whatever the provider reported', async () => {
    initialMessages = [{ ...message(1, 'assistant', 'Reported'), effort: 'high' }]
    setup()

    expect(await screen.findByText('Reported')).toBeInTheDocument()
    expect(screen.getAllByTestId('message-turn-actions')).toHaveLength(1)
    expect(screen.queryByText(/effort/i)).toBeNull()
  })

  it('never shows turn actions on a USER message', async () => {
    initialMessages = [{ ...message(1, 'user', 'Ask'), effort: 'high' }]
    setup()

    expect(await screen.findByText('Ask')).toBeInTheDocument()
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
  })
})

// A transcript carries four roles, and two of them are text NOBODY in the
// conversation typed. The row used to discriminate on a single `user` boolean, so
// a harness injection landed in the ledger — and on screen — as the user's own
// message.
describe('AgentChatView non-conversational roles', () => {
  // taskNotification is what claude 2.1.234's harness actually injected, captured
  // verbatim from raw hook stdin. It is used here rather than a placeholder
  // because its SHAPE is the point: it is markup, and a markdown renderer treats
  // `<task-notification>` as an HTML tag and swallows the row.
  const taskNotification =
    '<task-notification>\n<task-id>aa3b60603214670cc</task-id>\n' +
    '<status>completed</status>\n<result>PONG</result>\n</task-notification>'

  function roleMessage(sequence: number, role: string, text: string): AgentChatMessage {
    return {
      ...message(sequence, 'assistant', text),
      // Cast at the boundary on purpose: the third case below is a role this
      // build does not know, which is exactly what a newer daemon sends.
      role: role as AgentChatMessage['role'],
    }
  }

  it('renders a harness injection as plainly not the user', async () => {
    initialMessages = [roleMessage(1, 'harness', taskNotification)]
    setup()

    const row = await screen.findByTestId('agent-message-1')
    expect(row).toHaveAttribute('data-role', 'harness')
    // Not the user's bubble: the user's row is right-aligned and rounded into the
    // bottom-right corner, and this one is neither.
    expect(row.className).toContain('row')
    expect(row.className).not.toContain('me')
    expect(row.innerHTML).not.toContain('bubble')
    // Said in words, not only in styling.
    expect(screen.getByTestId('message-harness-label')).toHaveTextContent(/not by you/i)
    // And verbatim: the tags survive, which they would not through markdown.
    expect(screen.getByText(/<task-notification>/)).toBeInTheDocument()
  })

  it('announces a notice in the warning family the rest of the chat already uses', async () => {
    initialMessages = [
      roleMessage(
        1,
        'notice',
        "You've hit your usage limit. Your limits will reset at Aug 22nd, 2026 12:30 PM.",
      ),
    ]
    setup()

    expect(await screen.findByText(/hit your usage limit/i)).toBeInTheDocument()
    const alert = screen.getByRole('alert')
    // The same warning family the terminal-wait banner and the interruption strip
    // wear — carried by the stylesheet the artboards were lifted from.
    expect(alert.className).toContain('halted')
    // Never the user's bubble.
    // The provider's stop reason occupies the BAR, not a transcript row: it is
    // the reason there is nothing to type into, and saying it in both places
    // reads as the provider having stopped twice.
    expect(screen.queryByTestId('agent-message-1')).not.toBeInTheDocument()
  })

  it('still renders a role this build has never heard of', async () => {
    // A newer daemon minting a role must not make a turn the agent acted on
    // disappear from the transcript.
    initialMessages = [roleMessage(1, 'summary', 'a role from the future')]
    setup()

    expect(await screen.findByText('a role from the future')).toBeInTheDocument()
    expect(screen.getByTestId('agent-message-1')).toHaveAttribute('data-role', 'summary')
  })
  // ── The compaction boundary ────────────────────────────────────────
  // REGRESSION: the divider component and the transcript's prop both existed and
  // neither was ever fed, so a compacted chat drew no line at all. Interruptions
  // and messages share ONE sequence space, which is the whole derivation.
  describe('compaction divider', () => {
    const compactionAt = (seq: number, detail = 'manual') => ({
      ...emptyActivity,
      interruptions: [
        {
          id: `i-${seq}`,
          turnId: '',
          seq,
          kind: 'compaction' as const,
          detail,
          at: '2026-08-16T00:00:00Z',
          resolvedAt: '2026-08-16T00:00:01Z',
        },
      ],
    })

    it('draws the rule above the first message that follows the compaction', async () => {
      initialMessages = [
        message(10, 'user', 'before the boundary'),
        message(20, 'assistant', 'after the boundary'),
      ]
      activityFn.mockResolvedValue(compactionAt(15))
      setup()

      const divider = await screen.findByTestId('agent-compaction-divider')
      expect(divider.textContent).toMatch(/compacted/i)
      // Above the LATER message: everything over the line is gone from the
      // model's context, everything under it is not.
      const after = screen.getByText('after the boundary')
      expect(divider.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    })

    it('draws nothing when the compaction is newer than every message', async () => {
      initialMessages = [message(10, 'user', 'the only message')]
      activityFn.mockResolvedValue(compactionAt(99))
      setup()

      expect(await screen.findByText('the only message')).toBeTruthy()
      // A rule under the newest message would put a boundary below the whole
      // conversation and read as if the chat had ended.
      expect(screen.queryByTestId('agent-compaction-divider')).toBeNull()
    })

    it('names an automatic compaction differently from one the user asked for', async () => {
      initialMessages = [message(10, 'user', 'older'), message(20, 'assistant', 'newer')]
      activityFn.mockResolvedValue(compactionAt(15, 'auto'))
      setup()

      const divider = await screen.findByTestId('agent-compaction-divider')
      expect(divider.textContent).toMatch(/compacted automatically/i)
    })
  })

  // REGRESSION: a provider's own failure notice ("model_not_found") rendered
  // AFTER a later "Switched to Claude" divider — as if the OLD provider's
  // error belonged to the NEW session. The failure happened, and was fully
  // recorded, strictly BEFORE the switch (its sequence is lower), so the row
  // must sit above the divider that follows it.
  describe('a stale provider failure and a later switch', () => {
    it('keeps the failure notice ABOVE the switch that came after it', async () => {
      initialMessages = [
        roleMessage(1, 'notice', "There's an issue with the selected model (gpt-5.4-mini)."),
      ]
      activityFn.mockResolvedValue({
        ...emptyActivity,
        interruptions: [
          {
            id: 'sw-1',
            turnId: '',
            seq: 2,
            kind: 'provider_switched' as const,
            detail: 'claude',
            at: '2026-08-16T00:00:02Z',
            resolvedAt: '2026-08-16T00:00:02Z',
          },
        ],
      })
      // A message after the switch is what gives its divider something to
      // anchor before, and what stops the notice being the LAST message —
      // haltedBy suppresses a trailing notice into the bar, not a row, so
      // without this the notice would never reach the transcript at all.
      initialMessages.push(message(3, 'assistant', 'hi from claude', 'claude'))
      setup()

      const notice = await screen.findByText(/model_not_found|gpt-5\.4-mini/i)
      const divider = await screen.findByTestId('agent-event-divider')
      expect(divider.textContent).toMatch(/Switched to Claude/i)
      expect(
        notice.compareDocumentPosition(divider) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    })
  })
})

describe('AgentChatView surface hotswap', () => {
  it('hides the view switcher entirely when the provider has no terminal', async () => {
    setup({ providers: [{ ...providers[0], hasTerminal: false }, providers[1]] })

    await composer() // wait for the first paint past the ledger load
    expect(screen.queryByRole('tablist', { name: 'View' })).not.toBeInTheDocument()
  })

  it('shows the view switcher when the provider has a terminal', async () => {
    setup({ providers: [{ ...providers[0], hasTerminal: true }, providers[1]] })

    expect(await screen.findByRole('tablist', { name: 'View' })).toBeTruthy()
  })

  it('blocks handover to the terminal mid-turn when the provider cannot hotswap', async () => {
    setup({ providers: [{ ...providers[0], hotswap: false }, providers[1]], working: true })

    const terminalTab = await screen.findByRole('tab', { name: 'Terminal' })
    expect(terminalTab).toBeDisabled()
  })

  it('never blocks handover when the provider can hotswap, even mid-turn', async () => {
    setup({ providers: [{ ...providers[0], hotswap: true }, providers[1]], working: true })

    const terminalTab = await screen.findByRole('tab', { name: 'Terminal' })
    expect(terminalTab).not.toBeDisabled()
  })

  it('does not block handover when no turn is open, even without hotswap', async () => {
    setup({ providers: [{ ...providers[0], hotswap: false }, providers[1]], working: false })

    const terminalTab = await screen.findByRole('tab', { name: 'Terminal' })
    expect(terminalTab).not.toBeDisabled()
  })
})

// Design doc §5: "stop during the first turn" — split on whether the prompt
// had actually dispatched yet. The interruption itself is now a REAL backend
// activity record (turn.RecordStop), read back the same way a compaction
// boundary is — see the "stopped turn divider" describe block below for the
// sequence-anchored positioning that replaced the old local-only marker.
describe('AgentChatView first-turn stop', () => {
  // PRE-dispatch. The design doc names `prompts.remove` / `cancelUnsentPrompts`
  // as the mechanism this already goes through — QueuedRow's own Edit button
  // (prompts.remove + restore the draft) does exactly that, with nothing new
  // to build: a true undo, because nothing was ever recorded anywhere.
  it('reverses to the editable empty document, draft restored, when the first prompt is edited back before dispatch', async () => {
    setup({ working: true })
    await enterPrompt('the whole plan')
    expect(screen.getByTestId('queued-prompt')).toHaveAttribute('data-state', 'queued')
    expect(submitPromptFn).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Edit queued prompt' }))

    expect(await screen.findByTestId('agent-empty-document')).toBeInTheDocument()
    expect(await composer()).toHaveValue('the whole plan')
  })

  // POST-dispatch. The frozen document stays exactly where it is — no snap
  // back to blank, which would misreport a chat the backend still has a real,
  // resumable turn recorded against.
  it('keeps the frozen document in place across a stop', async () => {
    const view = setup({ working: false })
    await enterPrompt('build the feature')
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

    incrementalMessages = [message(1, 'user', 'build the feature')]
    view.rerenderProps({ working: true })
    await screen.findByTestId('agent-message-1')

    fireEvent.click(screen.getByRole('button', { name: 'Stop this turn' }))
    expect(stopChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(screen.queryByTestId('agent-empty-document')).not.toBeInTheDocument()

    view.rerenderProps({ working: false })
    await act(async () => Promise.resolve())

    expect(screen.getByTestId('agent-message-1')).toHaveAttribute('data-first-turn', 'true')
    expect(screen.queryByTestId('agent-empty-document')).not.toBeInTheDocument()
  })

  // REGRESSION, live-verified against the running app: stopping mid-generation
  // can make the CLI exit outright (not just pause), which drops `live` and
  // remounts the field from the pane's own pushed-in seed (`seedText`, driving
  // `initialValue` on the REAL, uncontrolled editor — see agent-chat-view.tsx's
  // `seed`) rather than from `draft`. Before Stop could fire with text still in
  // the box (composer-handle.tsx), that seed was always already "" at send
  // time, so a remount finding it stale was invisible. Now it can hold a real
  // follow-up, and a remount finding a STALE (pre-typing) seed there would
  // silently drop it — visibly, on the real editor; this harness's mocked
  // textarea can't lose text across a remount the way the real one does, so
  // the regression this proves is narrower: the remount `handleStop` forces
  // must carry the CURRENT text as its `initialValue`, not whatever the seed
  // last held before the follow-up was typed.
  it('seeds a stop-triggered remount with the just-typed follow-up, not a stale value', async () => {
    const view = setup({ working: false })
    await enterPrompt('build the feature')
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

    incrementalMessages = [message(1, 'user', 'build the feature')]
    view.rerenderProps({ working: true })
    await screen.findByTestId('agent-message-1')

    const inputBefore = await composer()
    fireEvent.change(inputBefore, { target: { value: 'wait, also check the date' } })
    fireEvent.click(screen.getByRole('button', { name: 'Stop this turn' }))

    // seedDraft always bumps draftSeed, forcing the (uncontrolled) editor to
    // remount even though nothing external pushed a genuinely new value — the
    // assertion that matters is what that fresh instance mounts holding.
    const inputAfter = await composer()
    expect(inputAfter).not.toBe(inputBefore)
    expect(inputAfter).toHaveValue('wait, also check the date')
  })
})

// REGRESSION: the composer's own stop button always advertised "Stop this
// turn — Esc" (composer-handle.tsx), but nothing ever wired Escape to the
// same action — handleKeyDown had no branch for it outside the slash-picker
// case, so it fell through and did nothing.
describe('AgentChatView Escape stops the turn', () => {
  it('stops the turn on Escape, exactly like the stop button, while a turn is working', async () => {
    setup({ working: true })
    const input = await composer()

    fireEvent.keyDown(input, { key: 'Escape' })

    expect(stopChatFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('does nothing on Escape while idle — there is no turn to stop', async () => {
    setup({ working: false })
    const input = await composer()

    fireEvent.keyDown(input, { key: 'Escape' })

    expect(stopChatFn).not.toHaveBeenCalled()
  })

  it('closes the slash picker on Escape in preference to stopping, when both are open', async () => {
    initialMessages = [message(1, 'assistant', 'earlier turn')]
    vi.useFakeTimers()
    slashCatalogFn.mockResolvedValue({
      providerId: 'claude',
      completeness: 'plugin_only',
      warnings: [],
      items: [],
    })
    setup({ working: true })
    const input = await composer()
    fireEvent.change(input, { target: { value: '/compact' } })
    await act(async () => vi.advanceTimersByTimeAsync(150))

    fireEvent.keyDown(input, { key: 'Escape' })

    expect(stopChatFn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })
})

// REGRESSION: the marker used to be LOCAL session state
// (firstTurnInterrupted), scoped to the first turn and pinned to "the end of
// the transcript" — so it drew nothing for a later turn's stop, and it kept
// sliding down under every message sent after the one it was supposed to
// mark. It is now a real, sequence-anchored `stopped` activity interruption
// (turn.RecordStop), positioned the exact same way a compaction boundary is.
describe('AgentChatView stopped turn divider', () => {
  const stoppedAt = (seq: number) => ({
    ...emptyActivity,
    interruptions: [
      {
        id: `stop-${seq}`,
        turnId: '',
        seq,
        kind: 'stopped' as const,
        detail: '',
        at: '2026-08-16T00:00:00Z',
        resolvedAt: '2026-08-16T00:00:01Z',
      },
    ],
  })

  it('marks where the turn was stopped, once it actually goes idle', async () => {
    const view = setup({ working: false })
    await enterPrompt('build the feature')
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

    incrementalMessages = [message(1, 'user', 'build the feature')]
    view.rerenderProps({ working: true })
    await screen.findByTestId('agent-message-1')

    fireEvent.click(screen.getByRole('button', { name: 'Stop this turn' }))
    // Still (nominally) working — the marker waits for the real idle edge
    // rather than assuming the click already succeeded.
    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()

    // The falling edge's own activity re-read is what would bring this back
    // from the real daemon; the mock stands in for that here.
    activityFn.mockResolvedValue(stoppedAt(1))
    view.rerenderProps({ working: false })

    expect(await screen.findByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
  })

  // Stopping a LATER turn is marked too — unlike the old first-turn-only
  // local state, this is the same architecture a mid-conversation compaction
  // already uses, and a compaction is never scoped to "only the first turn".
  it('marks a later turn’s stop too, anchored above the message that follows it', async () => {
    initialMessages = [message(1, 'user', 'first turn'), message(2, 'assistant', 'first reply')]
    activityFn.mockResolvedValue(stoppedAt(3))
    const view = setup({ working: false })
    await enterPrompt('second turn')
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

    incrementalMessages = [
      message(3, 'user', 'second turn'),
      message(4, 'assistant', 'third turn reply'),
    ]
    view.rerenderProps({ working: false, turnRevision: 2 })
    await screen.findByText('third turn reply')

    // Anchored above message 4 — the first one after the stop at seq 3 — not
    // trailing the whole transcript, and not absent just because it was not
    // the first turn.
    const divider = await screen.findByTestId('agent-interrupted-divider')
    const later = screen.getByText('third turn reply')
    expect(divider.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('does not move once a later message actually follows the stop', async () => {
    const view = setup({ working: false })
    await enterPrompt('build the feature')
    await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

    incrementalMessages = [message(1, 'user', 'build the feature')]
    view.rerenderProps({ working: true })
    await screen.findByTestId('agent-message-1')

    fireEvent.click(screen.getByRole('button', { name: 'Stop this turn' }))
    activityFn.mockResolvedValue(stoppedAt(1))
    view.rerenderProps({ working: false })
    await screen.findByTestId('agent-interrupted-divider')

    // Send and receive a SECOND message. The old, purely-local marker had no
    // sequence to anchor to and simply kept rendering after the newest thing
    // on screen — this one must hand off to a fixed position instead.
    incrementalMessages = [message(2, 'user', 'a second message')]
    view.rerenderProps({ working: true, turnRevision: 2 })
    await screen.findByText('a second message')
    view.rerenderProps({ working: false })

    const divider = await screen.findByTestId('agent-interrupted-divider')
    expect(screen.getAllByTestId('agent-interrupted-divider')).toHaveLength(1)
    const later = screen.getByText('a second message')
    expect(divider.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

describe('chat.open perf span', () => {
  beforeEach(() => {
    __resetPerfForTests()
    window.__CROWBAR_PERF__ = true
  })

  afterEach(() => {
    delete window.__CROWBAR_PERF__
  })

  it('closes the chat.open span once the first page of messages has loaded', async () => {
    initialMessages = [message(1, 'user', 'hello')]
    setup()

    await waitFor(() => {
      expect(performance.getEntriesByName('chat.open', 'measure')).toHaveLength(1)
    })
  })
})

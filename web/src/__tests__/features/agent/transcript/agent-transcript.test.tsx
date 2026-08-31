import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { AgentTranscript, ESTIMATED_ROW_HEIGHT, estimateRowHeight } from '@/features/agent/transcript/agent-transcript'
import type { TranscriptRow } from '@/features/agent/transcript/lib/flatten-transcript-rows'

// The historical rows are windowed (`@tanstack/react-virtual`), and jsdom has no
// layout engine: every element measures 0×0, and a virtualiser told its viewport
// is zero pixels tall windows down to NOTHING — `calculateRange` bails on
// `outerSize === 0` before overscan is ever applied, so not one row mounts.
// Give elements a pane-sized rect before render so the window under test is the
// realistic one the app produces. Purely geometric — no timers, no polling.
// Same stub, same reason as changed-files-tree.scale.test.tsx.
const VIEWPORT_WIDTH = 768
const VIEWPORT_HEIGHT = 800
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  const rect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

function draw(
  messages: AgentChatMessage[],
  overrides: Partial<Parameters<typeof AgentTranscript>[0]> = {},
) {
  return render(
    <AgentTranscript
      messages={messages}
      queue={[]}
      providers={[]}
      activity={{ toolCalls: [], subagents: [], interruptions: [], choices: [] }}
      working={false}
      loading={false}
      error={null}
      hasOlder={false}
      onLoadOlder={() => {}}
      onRetryLoad={() => {}}
      onOpenTerminal={() => {}}
      onEditPrompt={() => {}}
      onCancelPrompt={() => {}}
      onRetryPrompt={() => {}}
      {...overrides}
    />,
  )
}

// Whose agent answered now lives in the turnbar's own provider icon (see
// message-row.test.tsx), not a text label above the reply — the standalone
// "Claude" line this transcript used to draw is gone.
describe('AgentTranscript turnbar wiring', () => {
  it('gives every assistant reply its own turnbar, never a user message', () => {
    draw([
      { turnId: 't1', sequence: 1, role: 'user', providerId: '', text: 'hi', at: '' },
      { turnId: 't2', sequence: 2, role: 'assistant', providerId: 'claude', text: 'a', at: '' },
    ])

    expect(
      screen.getByTestId('agent-message-1').querySelector('[data-testid="message-turn-actions"]'),
    ).toBeNull()
    expect(
      screen.getByTestId('agent-message-2').querySelector('[data-testid="message-turn-actions"]'),
    ).not.toBeNull()
  })

  it("wires a turn's finished tool calls through to its own message row, keyed by turnId", () => {
    draw(
      [{ turnId: 't2', sequence: 1, role: 'assistant', providerId: 'claude', text: 'a', at: '' }],
      {
        activity: {
          toolCalls: [
            {
              id: 'c1',
              turnId: 't2',
              seq: 0,
              name: 'read_file',
              status: 'ok',
              hasRequest: true,
              hasResult: true,
              startedAt: '',
            },
          ],
          subagents: [],
          interruptions: [],
          choices: [],
        },
      },
    )

    expect(
      screen.getByTestId('agent-message-1').querySelector('[data-testid="agent-turn-tools"]'),
    ).not.toBeNull()
  })

  it('never gives a streaming bubble a turnbar or tool calls — the turn has not finished', () => {
    draw([], {
      streamingBubbles: [
        {
          turnId: 't1',
          sequence: 1,
          role: 'assistant',
          providerId: 'claude',
          text: 'typing…',
          at: '',
        },
      ],
      activity: {
        toolCalls: [
          {
            id: 'c1',
            turnId: 't1',
            seq: 0,
            name: 'read_file',
            status: 'ok',
            hasRequest: true,
            hasResult: true,
            startedAt: '',
          },
        ],
        subagents: [],
        interruptions: [],
        choices: [],
      },
    })

    expect(screen.getByText('typing…')).toBeInTheDocument()
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
    expect(screen.queryByTestId('agent-turn-tools')).toBeNull()
  })

  it("times a reply's turnbar against the user turn it answers, not against now", () => {
    draw([
      {
        turnId: 't1',
        sequence: 1,
        role: 'user',
        providerId: '',
        text: 'hi',
        at: '2026-08-24T00:00:00Z',
      },
      {
        turnId: 't1',
        sequence: 2,
        role: 'assistant',
        providerId: 'claude',
        text: 'a',
        at: '2026-08-24T00:04:00Z',
      },
    ])

    const turnbar = screen.getByTestId('agent-message-2').querySelector('.turnbar time')
    expect(turnbar).toHaveTextContent('4m')
  })

  it("skips a harness message between a user turn and the reply it answers — the harness's own words are not the user's", () => {
    draw([
      {
        turnId: 't1',
        sequence: 1,
        role: 'user',
        providerId: '',
        text: 'hi',
        at: '2026-08-24T00:00:00Z',
      },
      {
        turnId: 't1',
        sequence: 2,
        role: 'harness',
        providerId: 'claude',
        text: 'note',
        at: '2026-08-24T00:01:00Z',
      },
      {
        turnId: 't1',
        sequence: 3,
        role: 'assistant',
        providerId: 'claude',
        text: 'a',
        at: '2026-08-24T00:04:00Z',
      },
    ])

    const turnbar = screen.getByTestId('agent-message-3').querySelector('.turnbar time')
    expect(turnbar).toHaveTextContent('4m')
  })
})

describe('AgentTranscript first-turn framing', () => {
  it('freezes the oldest loaded message when there is nothing earlier to page in', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't2', sequence: 1, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
    ])

    expect(screen.getByTestId('agent-message-0')).toHaveAttribute('data-first-turn', 'true')
    expect(screen.getByTestId('agent-message-1')).not.toHaveAttribute('data-first-turn')
  })

  // The true first message is not always the first one ON SCREEN — paging in
  // older history must not make the CURRENT oldest-loaded message look frozen
  // when it never was the beginning of the conversation.
  it('does not freeze the oldest LOADED message when older history exists', () => {
    draw(
      [
        {
          turnId: 't5',
          sequence: 5,
          role: 'user',
          providerId: '',
          text: 'not actually first',
          at: '',
        },
      ],
      { hasOlder: true },
    )

    expect(screen.getByTestId('agent-message-5')).not.toHaveAttribute('data-first-turn')
  })

  it('draws the plain hairline right after the frozen turn, with no reply yet', () => {
    draw([{ turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' }])

    expect(screen.getByTestId('agent-first-turn-divider')).toBeInTheDocument()
  })

  it('draws no hairline at all once older history exists and nothing is frozen', () => {
    draw([{ turnId: 't5', sequence: 5, role: 'user', providerId: '', text: 'not first', at: '' }], {
      hasOlder: true,
    })

    expect(screen.queryByTestId('agent-first-turn-divider')).toBeNull()
  })

  // The assistant's answer to the frozen turn keeps its larger size too —
  // see message-row.tsx's own note on `firstReply`.
  it('marks the assistant message that answers the frozen turn, and no other', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't1', sequence: 1, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
      { turnId: 't2', sequence: 2, role: 'user', providerId: '', text: 'follow-up', at: '' },
      {
        turnId: 't2',
        sequence: 3,
        role: 'assistant',
        providerId: 'claude',
        text: 'second reply',
        at: '',
      },
    ])

    expect(screen.getByTestId('agent-message-1')).toHaveAttribute('data-first-reply', 'true')
    expect(screen.getByTestId('agent-message-3')).not.toHaveAttribute('data-first-reply')
  })

  it('does not mark anything when a harness message sits between the frozen turn and the reply', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't1', sequence: 1, role: 'harness', providerId: 'claude', text: 'note', at: '' },
      { turnId: 't1', sequence: 2, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
    ])

    // The harness row between them is never a candidate; the reply is still
    // the first assistant message after the frozen turn.
    expect(screen.getByTestId('agent-message-2')).toHaveAttribute('data-first-reply', 'true')
  })

  it('marks nothing when there is no reply yet', () => {
    draw([{ turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' }])

    expect(screen.queryByTestId('[data-first-reply]')).toBeNull()
  })

  it('marks nothing when older history exists, even if the oldest loaded message is an assistant reply', () => {
    draw(
      [
        {
          turnId: 't1',
          sequence: 5,
          role: 'assistant',
          providerId: 'claude',
          text: 'not actually the first reply',
          at: '',
        },
      ],
      { hasOlder: true },
    )

    expect(screen.getByTestId('agent-message-5')).not.toHaveAttribute('data-first-reply')
  })
})

describe('AgentTranscript windowed history', () => {
  function conversation(turns: number): AgentChatMessage[] {
    return Array.from({ length: turns }, (_, i) => ({
      turnId: `t${i}`,
      sequence: i,
      role: i % 2 === 0 ? ('user' as const) : ('assistant' as const),
      providerId: i % 2 === 0 ? '' : 'claude',
      text: `turn ${i}`,
      at: '',
    }))
  }

  // The whole point of the virtualiser: DOM cost bounded by the viewport, not
  // by the conversation. Node count rather than wall time deliberately — it is
  // deterministic, so this is a real gate instead of a flaky timing assertion.
  it('mounts a bounded window of rows, not one per message', () => {
    const { container } = draw(conversation(200))

    const mounted = container.querySelectorAll('.virtual-rows > [data-index]')
    expect(mounted.length).toBeGreaterThan(0)
    expect(mounted.length).toBeLessThan(40)
    expect(screen.getByTestId('agent-message-0')).toBeInTheDocument()
    expect(screen.queryByTestId('agent-message-199')).toBeNull()
  })

  // `.stream`'s `gap: 18px` fell between the per-message wrappers the old
  // render emitted — never inside one, so a divider always sat flush against
  // the message it belongs to. Absolutely-positioned virtual rows inherit no
  // gap at all, so it is baked into each row's own padding-bottom; this pins
  // that it still lands on group boundaries only.
  it('keeps the 18px between message groups, and nowhere inside one', () => {
    const { container } = draw(
      [
        { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
        { turnId: 't2', sequence: 1, role: 'assistant', providerId: 'claude', text: 'a', at: '' },
      ],
      { compactionBefore: { 1: 'manual' } },
    )

    const rows = Array.from(container.querySelectorAll<HTMLElement>('.virtual-rows > [data-index]'))
    // message 0, its first-turn hairline, the compaction boundary, message 1.
    expect(rows).toHaveLength(4)
    expect(rows[0].querySelector('[data-testid="agent-message-0"]')).not.toBeNull()
    expect(rows[1].querySelector('[data-testid="agent-first-turn-divider"]')).not.toBeNull()
    expect(rows[2].querySelector('[data-testid="agent-compaction-divider"]')).not.toBeNull()
    expect(rows[3].querySelector('[data-testid="agent-message-1"]')).not.toBeNull()

    // Flush under the message it trails; the gap goes after the group instead.
    expect(rows[0].style.paddingBottom).toBe('0px')
    expect(rows[1].style.paddingBottom).toBe('18px')
    // Flush above the message it leads, and nothing after the last row.
    expect(rows[2].style.paddingBottom).toBe('0px')
    expect(rows[3].style.paddingBottom).toBe('0px')
  })

  // REGRESSION coverage: the test above only ever exercises a message next to
  // a DIVIDER, never two ordinary messages back to back — which is the
  // spacing between essentially every pair of turns in a real conversation.
  // A bug where `endsMessageGroup` always returned `false` for a `message`
  // row would flatten every inter-message gap to zero and still pass that
  // test. `hasOlder: true` keeps a first-turn divider from interfering.
  it('puts the 18px gap between two ordinary messages with no divider between them', () => {
    const { container } = draw(conversation(3), { hasOlder: true })

    const rows = Array.from(container.querySelectorAll<HTMLElement>('.virtual-rows > [data-index]'))
    expect(rows).toHaveLength(3)
    expect(rows[0].querySelector('[data-testid="agent-message-0"]')).not.toBeNull()
    expect(rows[1].querySelector('[data-testid="agent-message-1"]')).not.toBeNull()
    expect(rows[2].querySelector('[data-testid="agent-message-2"]')).not.toBeNull()

    expect(rows[0].style.paddingBottom).toBe('18px')
    expect(rows[1].style.paddingBottom).toBe('18px')
    // Nothing after the last row.
    expect(rows[2].style.paddingBottom).toBe('0px')
  })

  // The composer is already showing this sentence; the transcript must not say
  // it twice. Survives the flattening — the whole group goes, dividers included.
  it('drops the row the composer is already showing', () => {
    draw(
      [
        { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
        { turnId: 't2', sequence: 1, role: 'user', providerId: '', text: 'being edited', at: '' },
      ],
      { suppressSequence: 1, interruptedBefore: { 1: true } },
    )

    expect(screen.getByTestId('agent-message-0')).toBeInTheDocument()
    expect(screen.queryByTestId('agent-message-1')).toBeNull()
    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()
  })
})

describe('AgentTranscript queued first turn', () => {
  let nextId = 0
  function queueItem(text: string) {
    nextId += 1
    return {
      clientRequestId: `r${nextId}`,
      text,
      state: 'queued' as const,
      createdAt: '2026-08-24T00:00:00Z',
      baselineSequence: 0,
    }
  }

  // A blank chat's first send is a QueuedRow, not yet a MessageRow — it has
  // to freeze on sight, before the provider ever confirms it, or sending
  // flashes through the dashed pending pill first.
  it('freezes the first queued item of a brand-new chat', () => {
    draw([], { queue: [queueItem('describe the change')] })

    const row = screen.getByTestId('queued-prompt')
    expect(row).toHaveAttribute('data-first-turn', 'true')
    expect(screen.getByTestId('agent-first-turn-divider')).toBeInTheDocument()
  })

  it('does not freeze a queued item once the chat already has history', () => {
    draw([{ turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' }], {
      queue: [queueItem('a follow-up')],
    })

    expect(screen.getByTestId('queued-prompt')).not.toHaveAttribute('data-first-turn')
  })

  it('does not freeze a queued item when older history has not been paged in yet', () => {
    draw([], { queue: [queueItem('not actually first')], hasOlder: true })

    expect(screen.getByTestId('queued-prompt')).not.toHaveAttribute('data-first-turn')
  })

  it('freezes only the head of the queue, never a second prompt queued behind it', () => {
    draw([], { queue: [queueItem('first'), queueItem('second')] })

    const rows = screen.getAllByTestId('queued-prompt')
    expect(rows[0]).toHaveAttribute('data-first-turn', 'true')
    expect(rows[1]).not.toHaveAttribute('data-first-turn')
  })

  // REGRESSION: `.queued` right-aligns via `align-self: flex-end`, which only
  // has any effect on a DIRECT flex child of `.stream`. A wrapping element
  // added around it (even one rendering nothing of its own) turns it into an
  // ordinary block instead — losing shrink-to-fit sizing and stretching it to
  // `max-width: 88%`, left-aligned inside that, which read as a large phantom
  // gap on the right. jsdom cannot see that layout consequence, but it CAN
  // see the DOM shape that causes it — so pin the shape, not the layout.
  it('keeps a queued row a direct child of .stream, not wrapped in an intermediate element', () => {
    const { container } = draw([], { queue: [queueItem('describe the change')] })

    const stream = container.querySelector('.stream')
    const row = screen.getByTestId('queued-prompt')
    expect(row.parentElement).toBe(stream)
  })
})

describe('AgentTranscript interrupted marker', () => {
  const oneFrozenTurn = [
    { turnId: 't1', sequence: 0, role: 'user' as const, providerId: '', text: 'first', at: '' },
  ]

  it('draws the trailing marker once the turn has actually gone idle, with nothing after it yet', () => {
    draw(oneFrozenTurn, { trailingInterruption: true, working: false })

    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
  })

  // The working line and the marker say opposite things about the same
  // instant — never both on screen, and the spinner's own disappearance is
  // what hands off to it, not a separate timer.
  it('does not draw the trailing marker while the turn still reads as working', () => {
    draw(oneFrozenTurn, { trailingInterruption: true, working: true })

    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()
  })

  it('draws nothing when nothing was interrupted', () => {
    draw(oneFrozenTurn, { trailingInterruption: false, working: false })

    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()
  })

  // REGRESSION: once a message actually follows the stop, the marker must hand
  // off to that FIXED position and stop trailing the transcript — the bug was
  // a marker that kept re-anchoring itself to "the end", drifting down every
  // time something new was said after the stop.
  it('draws anchored above the first message that follows the stop, not trailing, once one exists', () => {
    const messages = [
      { turnId: 't1', sequence: 0, role: 'user' as const, providerId: '', text: 'first', at: '' },
      {
        turnId: 't2',
        sequence: 1,
        role: 'user' as const,
        providerId: '',
        text: 'after the stop',
        at: '',
      },
    ]
    draw(messages, {
      interruptedBefore: { 1: true },
      trailingInterruption: false,
      working: false,
    })

    const divider = screen.getByTestId('agent-interrupted-divider')
    const later = screen.getByText('after the stop')
    expect(divider.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  // REGRESSION (live-reported 2026-08-30): interrupt a turn, switch provider, and
  // send a follow-up — the marker rendered AFTER the still-queued prompt (nothing
  // in `messages` outranks the interruption yet, so it fell through to the
  // trailing slot, which used to sit BELOW the queue) and only snapped above it
  // once the hook confirmed the prompt and moved it into `messages`. The marker
  // is part of "the record" the queue sits below, same as any confirmed message.
  it('draws the trailing marker above a prompt still waiting on hook confirmation', () => {
    draw(oneFrozenTurn, {
      trailingInterruption: true,
      working: false,
      queue: [
        {
          clientRequestId: 'r1',
          text: 'continue that essay',
          state: 'queued',
          createdAt: '2026-08-30T00:00:00Z',
          baselineSequence: 0,
        },
      ],
    })

    const divider = screen.getByTestId('agent-interrupted-divider')
    const queued = screen.getByTestId('queued-prompt')
    expect(divider.compareDocumentPosition(queued) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

describe('AgentTranscript switch marker', () => {
  const twoMessages = [
    { turnId: 't1', sequence: 0, role: 'user' as const, providerId: '', text: 'first', at: '' },
    { turnId: 't2', sequence: 1, role: 'user' as const, providerId: '', text: 'second', at: '' },
  ]

  it('draws a provider-switch divider, resolving the display name from the providers list', () => {
    draw(twoMessages, {
      switchesBefore: { 1: [{ what: 'provider', detail: 'codex' }] },
      providers: [{ id: 'codex', displayName: 'Codex' } as never],
    })

    expect(screen.getByTestId('agent-provider-switch-divider')).toHaveTextContent('Switched to Codex')
  })

  it('draws a model-changed divider with the raw model id', () => {
    draw(twoMessages, { switchesBefore: { 1: [{ what: 'model', detail: 'opus' }] } })

    expect(screen.getByTestId('agent-model-switch-divider')).toHaveTextContent('Model: opus')
  })

  it('draws an effort-changed divider with the raw effort level', () => {
    draw(twoMessages, { switchesBefore: { 1: [{ what: 'effort', detail: 'high' }] } })

    expect(screen.getByTestId('agent-effort-switch-divider')).toHaveTextContent('Effort: high')
  })

  it('draws both dividers, in order, when model and effort change together', () => {
    draw(twoMessages, {
      switchesBefore: {
        1: [
          { what: 'model', detail: 'opus' },
          { what: 'effort', detail: 'high' },
        ],
      },
    })

    const model = screen.getByTestId('agent-model-switch-divider')
    const effort = screen.getByTestId('agent-effort-switch-divider')
    expect(model.compareDocumentPosition(effort) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('draws nothing when nothing switched', () => {
    draw(twoMessages, {})

    expect(screen.queryByTestId('agent-provider-switch-divider')).toBeNull()
    expect(screen.queryByTestId('agent-model-switch-divider')).toBeNull()
    expect(screen.queryByTestId('agent-effort-switch-divider')).toBeNull()
  })
})

// Regression: a message settling from the streaming bubble (a real,
// unestimated DOM element) into this virtualized list used to start over at
// the SAME flat guess regardless of how long its own text was — a real,
// physical drop in total height for the one tick before measureElement
// corrected it, since the flat floor was sized for the shortest realistic
// reply. Visible live as the transcript glide up, then glide back down, on
// nearly every turn. Scaling the estimate off the row's own text shrinks
// that gap for the common case (prose of some length).
describe('estimateRowHeight', () => {
  function messageRow(text: string): TranscriptRow {
    return {
      kind: 'message',
      key: 'k',
      message: {
        turnId: 't',
        sequence: 0,
        role: 'assistant',
        providerId: 'claude',
        text,
        at: '',
      },
    }
  }

  it('never estimates below the flat floor, even for an empty message', () => {
    expect(estimateRowHeight(messageRow(''))).toBe(ESTIMATED_ROW_HEIGHT)
  })

  it('scales up for a message long enough to wrap several lines', () => {
    const short = estimateRowHeight(messageRow('a short reply'))
    const long = estimateRowHeight(messageRow('word '.repeat(400)))
    expect(short).toBe(ESTIMATED_ROW_HEIGHT)
    expect(long).toBeGreaterThan(short)
    // Genuinely taller, not a rounding nudge off the floor.
    expect(long).toBeGreaterThan(ESTIMATED_ROW_HEIGHT * 3)
  })

  it('a divider row always gets the flat floor — it has no text to scale from', () => {
    expect(estimateRowHeight({ kind: 'first-turn-divider', key: 'k' })).toBe(ESTIMATED_ROW_HEIGHT)
    expect(estimateRowHeight({ kind: 'interrupted-divider', key: 'k', sequence: 0 })).toBe(
      ESTIMATED_ROW_HEIGHT,
    )
    expect(
      estimateRowHeight({ kind: 'compaction-divider', key: 'k', sequence: 0, trigger: 'manual' }),
    ).toBe(ESTIMATED_ROW_HEIGHT)
  })
})

import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef } from 'react'
import { useVirtualizer, type Virtualizer } from '@tanstack/react-virtual'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type {
  AgentActivity,
  AgentChatMessage,
  AgentChoice,
  AgentProvider,
  AgentSubagent,
  AgentToolCall,
} from '@/features/agent/api/agent-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { samePrompt } from '@/features/agent/hooks/use-prompt-queue'
import { WorkingLine } from '@/features/agent/activity/working-line'
import {
  useTranscriptAnchor,
  type TranscriptScrollPosition,
} from '@/features/agent/hooks/use-transcript-anchor'
import { useScrollFrameSpan } from '@/features/agent/hooks/use-scroll-frame-span'
import { EventDivider } from '@/features/agent/transcript/event-divider'
import { FirstTurnDivider } from '@/features/agent/transcript/first-turn-divider'
import {
  flattenTranscriptRows,
  type DividerTag,
  type TranscriptRow,
} from '@/features/agent/transcript/lib/flatten-transcript-rows'
import { MessageRow } from '@/features/agent/transcript/message-row'
import { QueuedRow } from '@/features/agent/transcript/queued-row'
import {
  groupChoicesByTurn,
  groupSubagentsByTurn,
  groupToolCallsByTurn,
} from '@/features/agent/transcript/turn-tools'

interface AgentTranscriptProps {
  /** Needed only to fetch a finished tool call's own request/result bytes on
   *  demand — see MessageRow's own doc. */
  wsId?: string
  chatId?: string
  messages: AgentChatMessage[]
  /** One per still-open message item — see useChatMessages. Almost always
   *  0 or 1 entries; more than one only for a provider (Codex) that can
   *  split a turn's reply across several concurrent items. */
  streamingBubbles?: AgentChatMessage[]
  queue: PromptQueueItem[]
  providers: AgentProvider[]
  activity: AgentActivity
  working: boolean
  /** Is this chat LIVE mid-compaction right now — see WorkingLine's own prop
   *  doc for why this cannot come from `activity`. */
  compacting?: boolean
  /** What the agent is thinking right now — see WorkingLine's own prop doc. */
  reasoning?: string
  /** The running tool's live output — see WorkingLine's own prop doc. */
  toolOutput?: { id: string; text: string }
  /** The agent's own to-do list — see WorkingLine's own prop doc. */
  plan?: { text: string; status: string }[]
  loading: boolean
  error: Error | null
  hasOlder: boolean
  /** A message the composer is already showing, so the transcript does not say
   *  the same sentence twice. */
  suppressSequence?: number
  /** Everything that happened before the next message — a compaction, a
   *  stopped turn, a provider/model/effort switch — keyed by that message's
   *  own sequence, the row the merged divider draws above. A boundary is
   *  BETWEEN two messages, so the message that follows it is the only one
   *  that identifies it unambiguously. Several tags can share one anchor
   *  (a stop followed by a switch, or model+effort changing together) and
   *  draw as pills on the SAME wavy line rather than one divider each. */
  eventsBefore?: Record<number, DividerTag[]>
  /** The most recent `stopped`/`compaction` events with no later CONFIRMED
   *  message loaded yet — nothing to key them before, so they draw right
   *  after the last confirmed/streaming content instead: above any
   *  still-queued prompt too, which has no sequence yet and so can never
   *  anchor `eventsBefore` itself. */
  trailingInterruption?: DividerTag[]
  onLoadOlder: () => void
  onRetryLoad: () => void
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour, if any. */
  showTerminalHintFor?: string
  /**
   * The docked composer's own measured height, published as `--agent-dock-h`
   * by the caller (agent-chat-view.tsx) — see `TranscriptAnchor.notifyReflow`
   * for why this has to be threaded in rather than discovered locally: the
   * dock overlays this transcript via padding, not flex sizing, so growing
   * it changes `.scroll`'s `scrollHeight` without ever resizing `.scroll`
   * or its content, which is the one change this component's own resize
   * watching structurally cannot see.
   */
  dockHeight?: number
  /** A previously-saved scroll position for this exact chat, this session —
   *  see useTranscriptAnchor's own doc. Omitted or null: land at the
   *  bottom. */
  initialScrollPosition?: TranscriptScrollPosition | null
  /** Called once, on unmount, with wherever the reader ended up. */
  onScrollPositionChange?: (position: TranscriptScrollPosition) => void
}

/** The `at` of the user turn each assistant reply actually answers, keyed by
 *  the reply's own sequence — what the turnbar times its reply AGAINST,
 *  since "how long the agent took" means the gap from the prompt, not from
 *  now. One forward pass: the most recent user message seen so far is the
 *  one every assistant message until the next user message is answering. */
function precedingUserAtByAssistantSequence(messages: AgentChatMessage[]): Map<number, string> {
  const map = new Map<number, string>()
  let lastUserAt: string | undefined
  for (const message of messages) {
    if (message.role === 'user') lastUserAt = message.at
    else if (message.role === 'assistant' && lastUserAt !== undefined) {
      map.set(message.sequence, lastUserAt)
    }
  }
  return map
}

/** Which assistant replies are the LAST one before either a user turn or the
 *  end of the loaded window — auto mode lets the agent answer its own reply
 *  and keep going with no human in between, and each of those self-continued
 *  steps is a genuine, separate turn in the ledger (its own turnId, its own
 *  real elapsed time), not a fragment of one. But the turnbar (copy, elapsed
 *  time) reads as "a finished reply you might want to act on", and showing
 *  it on every intermediate step — when the agent is about to answer itself
 *  again with no pause for the reader — is what a screenshot flagged live as
 *  clutter, not signal: it should mark only the step that actually hands
 *  control back. A harness/notice row does not break the run (the agent did
 *  not stop for one), only a real user turn does — a backward pass one
 *  cheap way to ask "is a later assistant reply still coming before the next
 *  user turn". */
const EMPTY_SEQUENCE_SET: Set<number> = new Set()

function lastInAgentRunSequences(messages: AgentChatMessage[]): Set<number> {
  const last = new Set<number>()
  let sawAssistantSinceUser = false
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i]
    if (!message) continue
    if (message.role === 'user') {
      sawAssistantSinceUser = false
    } else if (message.role === 'assistant') {
      if (!sawAssistantSinceUser) last.add(message.sequence)
      sawAssistantSinceUser = true
    }
  }
  return last
}

/** `.stream`'s own `gap: 18px` (transcript.css), which an absolutely-positioned
 *  virtual row can never inherit — baked into the row box instead, where
 *  `measureElement` counts it as part of the row's height and the virtualizer's
 *  offsets stay right. */
const ROW_GAP = 18

/** An unmeasured row's opening guess FLOOR — a short assistant reply's real
 *  shape (padding + one prose line + turnbar + its own group gap), not 64,
 *  because a cold open's `scrollTop = scrollHeight` runs against this before
 *  anything is measured. Exported so the tests standing in for jsdom's
 *  missing layout engine can't drift from it. `estimateRowHeight` below
 *  scales up from here per row; this constant alone is what a divider or an
 *  empty/short message still gets. */
export const ESTIMATED_ROW_HEIGHT = 96

const ROW_PADDING = 8
const ROW_LINE_HEIGHT = 23
const ROW_TURNBAR = 30
// A rough, unverified "characters per wrapped prose line" for this column's
// measure — not measured against the real font, just enough to shrink a
// LONG message's estimate error from many lines to roughly the right
// handful, rather than pretending every message is exactly one line.
const CHARS_PER_LINE = 88

/**
 * A per-row estimate scaled by the message's own text length, not the flat
 * `ESTIMATED_ROW_HEIGHT` for every row regardless of content.
 *
 * Why this matters more than a marginal accuracy win: a message settling
 * from the streaming bubble (a real, unestimated DOM element, sized to its
 * actual content) into this virtualized list starts over at whatever this
 * function returns — and the flat floor was chosen as roughly the SHORTEST
 * realistic message's shape, so nearly every real reply (anything past one
 * line) landed shorter than it actually was. That gap is a REAL, physical
 * drop in `.stream`'s total height for the one tick between the row
 * appearing and `measureElement` correcting it — not something any
 * scroll-target trick can paper over, since the browser clamps `scrollTop`
 * to whatever `scrollHeight` actually is at that instant regardless of what
 * this hook wants it to be. Scaling the estimate off the same text this row
 * is about to render shrinks that gap for the common case (prose of some
 * length) even though it can't close it for every case (a table or an image
 * still surprises it) — which is what actually cuts the visible "glides up,
 * then glides back down" the flat floor produced on nearly every turn.
 */
export function estimateRowHeight(row: TranscriptRow): number {
  if (row.kind !== 'message') return ESTIMATED_ROW_HEIGHT
  const length = row.message.text.length
  const lines = Math.max(1, Math.ceil(length / CHARS_PER_LINE))
  return Math.max(
    ESTIMATED_ROW_HEIGHT,
    ROW_PADDING + lines * ROW_LINE_HEIGHT + ROW_TURNBAR + ROW_GAP,
  )
}

/**
 * Where the 18px actually went BEFORE this list was flattened.
 *
 * The old render emitted one `<div>` per MESSAGE, holding that message's own
 * dividers, and `.stream`'s flex `gap` fell between those wrappers — never
 * inside one. So a compaction/interrupted divider sat flush against the message
 * it leads, and a first-turn divider flush under the message it trails.
 * Flattening split each wrapper into separate rows, so the gap has to be
 * re-applied per GROUP rather than per row, or every divider would gain 18px of
 * air on both sides that it never had.
 */
function endsMessageGroup(rows: TranscriptRow[], index: number): boolean {
  const row = rows[index]
  if (row.kind === 'first-turn-divider') return true
  if (row.kind !== 'message') return false
  return rows[index + 1]?.kind !== 'first-turn-divider'
}

/**
 * tanstack's own `observeElementRect`, plus the pane-drag suppression
 * `use-agent-chat-list-virtualizer.ts` already proved out: while a sidebar/pane
 * drag is in progress (`data-pane-resizing` on `<html>`) hold the callback back
 * — otherwise the drag drives a re-render per frame — and flush the last
 * deferred measurement on `pane-resize-end`.
 *
 * Border-box, like the default: `.scroll` carries a padding-bottom the size of
 * the dock plus 120px, so a `contentRect` reading would under-report the
 * viewport by that much and window the transcript short of its own bottom edge.
 */
function observeScrollRect(
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
  cb: (rect: { width: number; height: number }) => void,
) {
  const element = instance.scrollElement
  if (!element) return
  const report = (rect: { width: number; height: number }) => {
    cb({ width: Math.round(rect.width), height: Math.round(rect.height) })
  }
  let pending: { width: number; height: number } | null = null
  const observer = new ResizeObserver(([entry]) => {
    const box = entry?.borderBoxSize?.[0]
    const rect = box
      ? { width: box.inlineSize, height: box.blockSize }
      : element.getBoundingClientRect()
    if (document.documentElement.hasAttribute('data-pane-resizing')) {
      pending = { width: rect.width, height: rect.height }
      return
    }
    pending = null
    report(rect)
  })
  observer.observe(element, { box: 'border-box' })
  report(element.getBoundingClientRect())
  const flush = () => {
    if (!pending) return
    report(pending)
    pending = null
  }
  window.addEventListener('pane-resize-end', flush)
  return () => {
    observer.disconnect()
    window.removeEventListener('pane-resize-end', flush)
  }
}

/**
 * One flattened row, drawn.
 *
 * A straight port of the `messages.map` block this replaced — same components,
 * same props, same order. `firstReply` is computed HERE rather than carried on
 * the row because `flattenTranscriptRows` deliberately does not know about it:
 * it only sets a presentational attribute on `MessageRow` and never gates a
 * row's presence or position, so it has no business in the row list's shape.
 */
function TranscriptRowView({
  row,
  providers,
  firstTurnSequence,
  firstReplySequence,
  callsByTurn,
  subagentsByTurn,
  choicesByTurn,
  wsId,
  chatId,
  precedingUserAt,
  lastInAgentRun,
}: {
  row: TranscriptRow
  providers: AgentProvider[]
  firstTurnSequence: number | undefined
  firstReplySequence: number | undefined
  callsByTurn: Map<string, AgentToolCall[]>
  subagentsByTurn: Map<string, AgentSubagent[]>
  choicesByTurn: Map<string, AgentChoice[]>
  wsId?: string
  chatId?: string
  precedingUserAt: Map<number, string>
  lastInAgentRun: Set<number>
}) {
  switch (row.kind) {
    case 'event-divider':
      return <EventDivider tags={row.tags} providers={providers} />
    case 'first-turn-divider':
      return <FirstTurnDivider />
    case 'message': {
      const assistant = row.message.role === 'assistant'
      return (
        <MessageRow
          message={row.message}
          providers={providers}
          firstTurn={row.message.sequence === firstTurnSequence}
          firstReply={row.message.sequence === firstReplySequence}
          turnbar={lastInAgentRun.has(row.message.sequence)}
          toolCallsByTurn={assistant ? callsByTurn : undefined}
          subagentsByTurn={assistant ? subagentsByTurn : undefined}
          choicesByTurn={assistant ? choicesByTurn : undefined}
          wsId={wsId}
          chatId={chatId}
          precedingUserAt={precedingUserAt.get(row.message.sequence)}
        />
      )
    }
  }
}

/**
 * The conversation.
 *
 * Bottom-anchored, because a chat is read from its newest end. Queued prompts sit
 * below the record and above the working line, which is the order they will
 * actually happen in.
 */
// This component's own effects are the load-bearing part of this branch's
// entire scroll/streaming correctness work (pin-to-top, tail-room, the
// streaming/queued row height cache above, prepend/restore-position) and are
// heavily interdependent through shared refs and measured DOM state, not a
// pile of unrelated concerns. Splitting it is a real architecture decision,
// not a quick fix — per this tool's own guidance ("split behavior-changing
// work into separate PRs"), and given how delicate this exact file's timing
// has already proven this session (see its own effects' doc comments), it
// belongs in its own reviewed pass, not a last change before a CI deadline.
// react-doctor-disable-next-line no-giant-component -- see comment above, splitting this is a separate architectural pass
export function AgentTranscript(props: AgentTranscriptProps) {
  const { messages, queue, dockHeight } = props
  const anchor = useTranscriptAnchor({
    loadingHistory: props.loading,
    initialPosition: props.initialScrollPosition,
    onPositionChange: props.onScrollPositionChange,
  })
  const scrollFrame = useScrollFrameSpan()
  // The dock overlays this transcript rather than sizing it (see
  // `TranscriptAnchor.notifyReflow`'s own doc), so a height change here is
  // invisible to the anchor's internal ResizeObservers — this is the only
  // signal that ever reaches it. Fires on mount too, which is a harmless,
  // idempotent no-op: the initial layout is already correct by construction.
  useEffect(() => {
    anchor.notifyReflow()
  }, [dockHeight, anchor.notifyReflow])
  // A TURN STARTING lifts the prompt that started it to the top of the
  // transcript, so the reply has the whole viewport to grow down into rather
  // than whatever slice bottom-following happened to leave under the previous
  // turn — see `TranscriptAnchor.pinTurnToTop`.
  //
  // The queue's newest `clientRequestId` is the signal, because it is the only
  // one that fires exactly ONCE per turn at the moment of dispatch:
  // `messages` lags by a poll, `working` lags the daemon and is true for
  // agent self-continued turns too, and `streamingBubbles` changes on every
  // token. `enqueue` pushes the item synchronously, so the queued row is in
  // the DOM by the time this layout effect reads for it.
  //
  // It is the QUEUED row that gets measured, not a message row: a just-sent
  // prompt has no ledger-confirmed row yet, and by the time it does the pin's
  // work is already done (`pinTurnToTop` keeps the offset, not the element).
  const pinnedRequestId = useRef<string | null>(null)
  // The prompt item the CURRENT pin was measured from, if any — kept so a
  // later run, once the queue drains it, can tell "this prompt settled into
  // the ledger, the pin is still exactly right" apart from "this prompt
  // vanished without ever sending" (canceled before it dispatched), which
  // needs an explicit release. See the empty-queue branch below.
  const pinnedItem = useRef<PromptQueueItem | null>(null)
  const sawFirstQueue = useRef(false)
  useLayoutEffect(() => {
    const newestItem = queue.at(-1) ?? null
    const newest = newestItem?.clientRequestId ?? null
    // A chat REOPENED with prompts still waiting inherits them; that is a
    // restore, not a send, so the first run only ever records what it found.
    //
    // Computed and flipped BEFORE the equality check below, not after: a
    // freshly-mounted pane's very first run starts with an EMPTY queue, so
    // `newest` (null) already equals `pinnedRequestId.current`'s own initial
    // value (also null) — the equality check below would return before ever
    // reaching this flip, leaving `sawFirstQueue.current` false forever. The
    // user's actual first send then finds `sawFirstQueue.current` still
    // false, reads its OWN run as "inherited", and never pins — silently
    // losing pin-to-top for the single most common case, the first prompt in
    // a chat's lifetime.
    const inherited = !sawFirstQueue.current
    sawFirstQueue.current = true
    if (newest === pinnedRequestId.current) return
    pinnedRequestId.current = newest
    if (inherited) return
    if (newest) {
      pinnedItem.current = newestItem
      const row = anchor.scrollRef.current?.querySelector<HTMLElement>(
        `[data-client-request-id="${CSS.escape(newest)}"]`,
      )
      if (row) anchor.pinTurnToTop(row)
      return
    }
    // The queue just drained to empty. If the prompt that was pinned settled
    // into a real ledger message, the pin is still exactly right — it keeps
    // an OFFSET, not the element, and releases itself once the reply grows
    // past the reserved room (see tailRoom). But if it vanished WITHOUT
    // settling — "Cancel unsent prompts", before it ever dispatched —
    // nothing will ever grow to fill that room: applyTailRoom's own
    // shortfall math then reads the now-SHRUNKEN content as needing MORE
    // reserved space, not less, and grows a permanent, ever-widening blank
    // gap instead of releasing it. `pinTurnToTop(null)` — the documented
    // release path — is the only way out of that once it has happened, and
    // nothing else in this file ever calls it.
    const settled = pinnedItem.current && messages.some((m) => samePrompt(m, pinnedItem.current!))
    if (!settled) anchor.pinTurnToTop(null)
    pinnedItem.current = null
  }, [queue, messages, anchor.scrollRef, anchor.pinTurnToTop])
  const callsByTurn = useMemo(
    () => groupToolCallsByTurn(props.activity.toolCalls),
    [props.activity.toolCalls],
  )
  const subagentsByTurn = useMemo(
    () => groupSubagentsByTurn(props.activity.subagents),
    [props.activity.subagents],
  )
  const choicesByTurn = useMemo(
    () => groupChoicesByTurn(props.activity.choices),
    [props.activity.choices],
  )
  const precedingUserAt = useMemo(() => precedingUserAtByAssistantSequence(messages), [messages])
  // Empty while `working` — the settled reply this would otherwise mark is not
  // actually the run's last step any more the instant the agent starts on the
  // next one (self-continued or freshly prompted; `working` covers both, see
  // this file's own note on it above). Without this a screenshot showed the
  // turnbar staying persistent on a reply the agent had already moved past.
  const lastInAgentRun = useMemo(
    () => (props.working ? EMPTY_SEQUENCE_SET : lastInAgentRunSequences(messages)),
    [messages, props.working],
  )
  // The ABSOLUTE first turn, never the first one merely loaded — `hasOlder`
  // paging in more history must not retroactively unfreeze a message that was
  // never actually the beginning of the conversation. Only meaningful once
  // there is nothing earlier to page in.
  const firstTurnSequence = !props.hasOlder ? messages[0]?.sequence : undefined
  // The assistant's answer to that frozen turn — kept in the same larger
  // hand rather than dropping to ordinary reply prose the instant the turn
  // ends. Tied to `firstTurnSequence` existing at all, not merely to being
  // the first assistant message loaded — same reasoning as above.
  const firstReplySequence =
    firstTurnSequence !== undefined
      ? messages.find((m) => m.role === 'assistant' && m.sequence > firstTurnSequence)?.sequence
      : undefined

  // The historical record, flat and windowed. Only the slice near the viewport
  // (plus overscan) is ever mounted, so a thousand-turn chat costs the same DOM
  // as a ten-turn one. Everything AFTER this block — the trailing interruption,
  // the streaming bubbles, the queue, the working line — stays an ordinary flex
  // child: small-count, always-visible tail items, and leaving them alone is
  // what keeps `.stream`'s `margin-top: auto` bottom-anchor (and so
  // `use-transcript-anchor.ts`) working exactly as before.
  const rows = useMemo(
    () =>
      flattenTranscriptRows({
        messages,
        eventsBefore: props.eventsBefore,
        firstTurnSequence,
        suppressSequence: props.suppressSequence,
      }),
    [messages, props.eventsBefore, firstTurnSequence, props.suppressSequence],
  )
  // By row key, not index: paging older messages in prepends rows, and an
  // index-keyed measurement cache would hand every row the height of whatever
  // used to sit at its position.
  const getItemKey = useCallback((index: number) => rows[index]?.key ?? index, [rows])
  const rowVirtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: rows.length,
    getScrollElement: () => anchor.scrollRef.current,
    estimateSize: (index) => estimateRowHeight(rows[index]),
    overscan: 12,
    measureElement: (el) => el.getBoundingClientRect().height,
    getItemKey,
    observeElementRect: observeScrollRect,
    // Off by design, not a default left alone. `measureElement`'s ref fires
    // during React's own commit phase — a lifecycle callback — and whenever a
    // row's real size differs enough from its estimate to need a scroll
    // compensation (routine here: rows vary from a one-line reply to a table,
    // ESTIMATED_ROW_HEIGHT is a single guess), react-virtual's default
    // (`useFlushSync: true`) calls `flushSync` from inside that same
    // callback — forcing a synchronous re-render while React is already
    // mid-commit. That's the exact "flushSync was called from inside a
    // lifecycle method" React throws — seen live in this app's console
    // clustered around a message settling from the interactive streaming
    // editor to MarkdownMessageStatic (message-row.tsx), itself a commit
    // landing at the same moment nearby rows get their first real
    // measurement. `false`
    // routes the same update through a plain `useReducer` dispatch instead —
    // identical scroll-adjustment math (virtual-core's `resizeItem` /
    // `applyScrollAdjustment`), just learned about asynchronously, which is
    // what removes the conflict rather than papering over its symptom.
    useFlushSync: false,
  })

  // The streaming bubble's own LAST REAL height, by message sequence — kept
  // only as long as that message is actually streaming. Read once, in the
  // settle effect below, the moment that same sequence reappears as a
  // virtualized row: `estimateRowHeight` has to guess from raw character
  // count alone, and for anything its line-height model doesn't fit — a
  // heading, a list, a table — that guess lands well short of a real reply's
  // height. This is the one case a guess is unnecessary: the content just sat
  // on screen, laid out for real, a moment before the same message settles
  // into the virtualizer. Reusing that measurement instead of re-guessing is
  // what closes the "glides up, then drops hard, then glides back up" gap
  // `estimateRowHeight`'s own doc comment already describes as a residual,
  // physical drop in `.stream`'s height the browser clamps `scrollTop`
  // against — measured live: a 212px hard drop, then a ~230ms climb back.
  const lastStreamedHeight = useRef(new Map<number, number>())
  useLayoutEffect(() => {
    const bubbles = props.streamingBubbles
    const container = anchor.scrollRef.current
    if (!bubbles?.length || !container) return
    for (const bubble of bubbles) {
      const el = container.querySelector<HTMLElement>(`[data-sequence="${bubble.sequence}"]`)
      if (el) lastStreamedHeight.current.set(bubble.sequence, el.getBoundingClientRect().height)
    }
    // Deliberately gated on `streamingBubbles` alone, not every render: this
    // pays a querySelector + forced-synchronous getBoundingClientRect per
    // streaming bubble, and AgentTranscript re-renders on every rAF-batched
    // token flush while a reply is actively streaming — ungated, this
    // reintroduced exactly the per-frame layout cost the rest of this
    // branch exists to remove.
    // `anchor.scrollRef` is a ref: `.current` is read fresh when the effect
    // body runs regardless of the deps array, so it is never "stale" the way
    // a plain value could be, and including the (identity-stable) ref object
    // itself would change nothing — React's own documented exemption.
    // react-doctor-disable-next-line exhaustive-deps -- see comment above, anchor.scrollRef is a ref
  }, [props.streamingBubbles])
  // Primes the virtualizer with that real height BEFORE this row's first
  // paint as a virtualized item, rather than letting it start from
  // `estimateRowHeight`'s guess and wait for `measureElement` to correct it a
  // beat later. One-shot per message: the cache entry is consumed (deleted)
  // the instant it is used, so a later, ordinary re-measurement of the same
  // row (content still settling, a code block highlighting in) is untouched.
  useLayoutEffect(() => {
    if (lastStreamedHeight.current.size === 0) return
    rows.forEach((row, index) => {
      if (row.kind !== 'message') return
      const cached = lastStreamedHeight.current.get(row.message.sequence)
      if (cached === undefined) return
      lastStreamedHeight.current.delete(row.message.sequence)
      rowVirtualizer.resizeItem(index, cached)
    })
  }, [rows, rowVirtualizer])

  // The queued row's own LAST REAL height, by clientRequestId — the same
  // idea as `lastStreamedHeight` above, one step earlier in a message's
  // life. Dispatch removes a prompt's `QueuedRow` from `.stream` the
  // instant the daemon confirms it, well before the corresponding message
  // is necessarily back in `rows` (that needs its own fetch or WS push) —
  // so the real height that row's own prompt text was occupying vanishes
  // outright for however long that gap lasts, and `estimateRowHeight`'s
  // guess stands in until `measureElement` corrects it a beat later.
  // Measured live: a 245px hard drop the instant the queued row unmounts,
  // then a climb back — reported as "bouncing... once the provider
  // approved and confirmed the message has been submitted".
  //
  // Matched via `samePrompt` — the SAME evidence usePromptQueue itself
  // trusts to retire a queued item — rather than a second, driftable
  // definition of "is this THAT prompt" living here.
  const lastQueuedHeight = useRef(new Map<string, { item: PromptQueueItem; height: number }>())
  useLayoutEffect(() => {
    const container = anchor.scrollRef.current
    if (!container) return
    for (const item of queue) {
      const el = container.querySelector<HTMLElement>(
        `[data-client-request-id="${CSS.escape(item.clientRequestId)}"]`,
      )
      if (el)
        lastQueuedHeight.current.set(item.clientRequestId, {
          item,
          height: el.getBoundingClientRect().height,
        })
    }
    // Gated on `queue` alone — see the streaming-bubble effect above's own
    // comment for why: the same per-item forced-layout cost, paid on every
    // render instead of only when the queue actually changes.
    // Same false positive as the streaming-bubble effect above:
    // `anchor.scrollRef` is a ref, and reading `.current` inside the effect
    // body is never stale.
    // react-doctor-disable-next-line exhaustive-deps -- see comment above, anchor.scrollRef is a ref
  }, [queue])
  // Primes the virtualizer the same way the streaming-bubble effect above
  // does, for the same reason. One-shot per prompt: consumed (deleted) the
  // instant a match is used, so a later, ordinary re-measurement of the
  // same row is untouched.
  useLayoutEffect(() => {
    if (lastQueuedHeight.current.size === 0) return
    rows.forEach((row, index) => {
      if (row.kind !== 'message') return
      for (const [key, { item, height }] of lastQueuedHeight.current) {
        if (!samePrompt(row.message, item)) continue
        lastQueuedHeight.current.delete(key)
        rowVirtualizer.resizeItem(index, height)
        break
      }
    })
  }, [rows, rowVirtualizer])

  return (
    <div
      className="scroll"
      data-testid="agent-message-list"
      ref={anchor.scrollRef}
      onScroll={() => {
        anchor.onScroll()
        scrollFrame.onScrollEvent()
      }}
    >
      {/* Bottom-anchor for a SHORT conversation — see transcript.css's own
          comment on `.scroll-spacer` for why this is a separate flex-grow
          sibling and not `margin-top: auto` on `.stream` itself. */}
      <div className="scroll-spacer" aria-hidden="true" />
      <div className="center stream">
        {props.hasOlder && (
          <Button
            className="self-center"
            variant="ghost"
            size="sm"
            onClick={() => {
              anchor.preservePosition()
              props.onLoadOlder()
            }}
          >
            Load earlier messages
          </Button>
        )}
        {props.loading && messages.length === 0 && (
          <div className="flex flex-1 items-center justify-center gap-2 text-muted-foreground text-sm">
            <FlickerSpinner className="size-4" /> Loading messages…
          </div>
        )}
        {props.error && messages.length === 0 && (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
            <p>Couldn’t load this chat’s messages.</p>
            <div className="flex gap-2">
              <Button size="sm" variant="secondary" onClick={props.onRetryLoad}>
                Retry
              </Button>
              <Button size="sm" variant="ghost" onClick={props.onOpenTerminal}>
                <TerminalIcon /> Terminal
              </Button>
            </div>
          </div>
        )}
        {/* Rendered only when there is something to render: an empty wrapper is
            still a flex child, and `.stream`'s `gap` would put 18px of nothing
            between a zero-height box and whatever follows it — the old
            `messages.map` over an empty array emitted no element at all. */}
        {rows.length > 0 && (
          <div className="virtual-rows" style={{ height: `${rowVirtualizer.getTotalSize()}px` }}>
            {rowVirtualizer.getVirtualItems().map((virtualRow) => {
              const row = rows[virtualRow.index]
              if (!row) return null
              const gapAfter =
                virtualRow.index < rows.length - 1 && endsMessageGroup(rows, virtualRow.index)
                  ? ROW_GAP
                  : 0
              return (
                <div
                  key={row.key}
                  ref={rowVirtualizer.measureElement}
                  data-index={virtualRow.index}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                    paddingBottom: gapAfter,
                  }}
                >
                  <TranscriptRowView
                    row={row}
                    providers={props.providers}
                    firstTurnSequence={firstTurnSequence}
                    firstReplySequence={firstReplySequence}
                    callsByTurn={callsByTurn}
                    subagentsByTurn={subagentsByTurn}
                    choicesByTurn={choicesByTurn}
                    wsId={props.wsId}
                    chatId={props.chatId}
                    precedingUserAt={precedingUserAt}
                    lastInAgentRun={lastInAgentRun}
                  />
                </div>
              )
            })}
          </div>
        )}
        {props.trailingInterruption &&
          props.trailingInterruption.length > 0 &&
          !props.working &&
          !props.compacting && (
            <EventDivider tags={props.trailingInterruption} providers={props.providers} />
          )}
        {props.streamingBubbles?.map((bubble) => (
          <MessageRow
            key={bubble.sequence}
            message={bubble}
            providers={props.providers}
            streaming
          />
        ))}
        {queue.map((item, index) => {
          // The ABSOLUTE first turn, exactly as firstTurnSequence reasons about
          // it above: nothing loaded yet, nothing older to page in, and this is
          // the head of the queue — the one prompt that is about to BECOME
          // sequence zero, not merely the first one currently waiting.
          const isFirstTurn = index === 0 && messages.length === 0 && !props.hasOlder
          // A Fragment, NOT a div: `.queued` aligns itself via `align-self:
          // flex-end` on the assumption that it is a DIRECT flex child of
          // `.stream` — a wrapping element (even an empty-looking one) makes
          // it a normal block instead, which drops the shrink-to-fit sizing
          // flex items get and stretches it to `max-width: 88%` every time,
          // left-aligned inside that. A Fragment adds no box, so `.queued`
          // stays a real flex child same as it was before this needed a
          // sibling to hold the divider.
          return (
            <Fragment key={item.clientRequestId}>
              <QueuedRow
                item={item}
                firstTurn={isFirstTurn}
                onEdit={() => props.onEditPrompt(item)}
                onCancel={() => props.onCancelPrompt(item.clientRequestId)}
                onRetry={() => props.onRetryPrompt(item.clientRequestId)}
                showTerminalHint={props.showTerminalHintFor === item.clientRequestId}
                onOpenTerminal={props.onOpenTerminal}
              />
              {isFirstTurn && <FirstTurnDivider />}
            </Fragment>
          )
        })}
        <WorkingLine
          activity={props.activity}
          working={props.working}
          since={messages.at(-1)?.at}
          compactingLive={props.compacting}
          reasoning={props.reasoning}
          toolOutput={props.toolOutput}
          plan={props.plan}
        />
        {/* A REAL, measured spacer — not `.scroll`'s own `padding-bottom` (see
            `.dock-spacer`'s own comment in transcript.css for why: the
            reservation has to be part of `.stream`'s actual, natural content
            height for `.scroll`'s scrollHeight to unambiguously include it). */}
        <div className="dock-spacer" aria-hidden="true" />
      </div>
    </div>
  )
}

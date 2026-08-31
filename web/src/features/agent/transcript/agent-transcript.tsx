import { Fragment, useCallback, useMemo } from 'react'
import { useVirtualizer, type Virtualizer } from '@tanstack/react-virtual'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type {
  AgentActivity,
  AgentChatMessage,
  AgentProvider,
  AgentToolCall,
} from '@/features/agent/api/agent-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { WorkingLine } from '@/features/agent/activity/working-line'
import { useTranscriptAnchor } from '@/features/agent/hooks/use-transcript-anchor'
import { useScrollFrameSpan } from '@/features/agent/hooks/use-scroll-frame-span'
import { CompactionDivider } from '@/features/agent/transcript/compaction-divider'
import { FirstTurnDivider } from '@/features/agent/transcript/first-turn-divider'
import { InterruptedDivider } from '@/features/agent/transcript/interrupted-divider'
import {
  flattenTranscriptRows,
  type TranscriptRow,
} from '@/features/agent/transcript/lib/flatten-transcript-rows'
import { SwitchDivider, type SwitchKind } from '@/features/agent/transcript/switch-divider'
import { MessageRow } from '@/features/agent/transcript/message-row'
import { QueuedRow } from '@/features/agent/transcript/queued-row'
import { groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'

interface AgentTranscriptProps {
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
  loading: boolean
  error: Error | null
  hasOlder: boolean
  /** A message the composer is already showing, so the transcript does not say
   *  the same sentence twice. */
  suppressSequence?: number
  /** The compaction boundary, keyed by the sequence of the first message AFTER
   *  it — the row the divider is drawn above. A compaction is a boundary
   *  BETWEEN two messages, so the message that follows it is the only one that
   *  identifies it unambiguously. */
  compactionBefore?: Record<number, 'manual' | 'auto' | string>
  /** A stopped turn, keyed the same way compactionBefore is: by the sequence of
   *  the first message AFTER it, the row the divider draws above. */
  interruptedBefore?: Record<number, true>
  /** Provider/model/effort switches — Crowbar's own doing, keyed the same way
   *  compactionBefore is. An ARRAY per sequence: a single SetChatSelection
   *  call can change model and effort together, and both get their own
   *  divider before the same next message. */
  switchesBefore?: Record<number, Array<{ what: SwitchKind; detail: string }>>
  /** The most recent stop with no later CONFIRMED message loaded yet — nothing
   *  to key it before, so it draws right after the last confirmed/streaming
   *  content instead: above any still-queued prompt too, which has no
   *  sequence yet and so can never anchor `interruptedBefore` itself. */
  trailingInterruption?: boolean
  onLoadOlder: () => void
  onRetryLoad: () => void
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour, if any. */
  showTerminalHintFor?: string
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
  precedingUserAt,
}: {
  row: TranscriptRow
  providers: AgentProvider[]
  firstTurnSequence: number | undefined
  firstReplySequence: number | undefined
  callsByTurn: Map<string, AgentToolCall[]>
  precedingUserAt: Map<number, string>
}) {
  switch (row.kind) {
    case 'compaction-divider':
      return <CompactionDivider trigger={row.trigger} />
    case 'interrupted-divider':
      return <InterruptedDivider />
    case 'switch-divider':
      return <SwitchDivider what={row.what} detail={row.detail} providers={providers} />
    case 'first-turn-divider':
      return <FirstTurnDivider />
    case 'message':
      return (
        <MessageRow
          message={row.message}
          providers={providers}
          firstTurn={row.message.sequence === firstTurnSequence}
          firstReply={row.message.sequence === firstReplySequence}
          toolCallsByTurn={row.message.role === 'assistant' ? callsByTurn : undefined}
          precedingUserAt={precedingUserAt.get(row.message.sequence)}
        />
      )
  }
}

/**
 * The conversation.
 *
 * Bottom-anchored, because a chat is read from its newest end. Queued prompts sit
 * below the record and above the working line, which is the order they will
 * actually happen in.
 */
export function AgentTranscript(props: AgentTranscriptProps) {
  const { messages, queue } = props
  const anchor = useTranscriptAnchor()
  const scrollFrame = useScrollFrameSpan()
  const callsByTurn = useMemo(
    () => groupToolCallsByTurn(props.activity.toolCalls),
    [props.activity.toolCalls],
  )
  const precedingUserAt = useMemo(() => precedingUserAtByAssistantSequence(messages), [messages])
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
        compactionBefore: props.compactionBefore,
        interruptedBefore: props.interruptedBefore,
        switchesBefore: props.switchesBefore,
        firstTurnSequence,
        suppressSequence: props.suppressSequence,
      }),
    [
      messages,
      props.compactionBefore,
      props.interruptedBefore,
      props.switchesBefore,
      firstTurnSequence,
      props.suppressSequence,
    ],
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
                    precedingUserAt={precedingUserAt}
                  />
                </div>
              )
            })}
          </div>
        )}
        {props.trailingInterruption && !props.working && <InterruptedDivider />}
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
        />
      </div>
    </div>
  )
}

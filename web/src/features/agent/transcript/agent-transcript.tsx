import {
  Fragment,
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useVirtualizer, type Virtualizer } from '@tanstack/react-virtual'
import { layoutHeight } from '@/lib/layout-size'
import { markEnd } from '@/lib/perf/instrumentation'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { WorkingLine } from '@/features/agent/activity/working-line'
import { useChatItems, useChatOrder, useLiveTail } from '@/features/agent/hooks/use-chat-feed'
import { useScrollFrameSpan } from '@/features/agent/hooks/use-scroll-frame-span'
import {
  useTranscriptAnchor,
  type TranscriptScrollPosition,
} from '@/features/agent/hooks/use-transcript-anchor'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import type { ChatFeed, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'
import { FirstTurnDivider } from '@/features/agent/transcript/first-turn-divider'
import { QueuedRow } from '@/features/agent/transcript/queued-row'
import { TranscriptRow, type RowPlace } from '@/features/agent/transcript/rows/transcript-row'

interface AgentTranscriptProps {
  chatId: string
  providers: AgentProvider[]
  queue: PromptQueueItem[]
  status?: ChatFeed['status']
  working: boolean
  /** When the running turn started. */
  since?: string
  compacting: boolean
  reasoning?: string
  plan?: { text: string; status?: string }[]
  /** An ask holds the composer. */
  asking: boolean
  /** A row the composer already shows (the provider's halt notice): said once. */
  suppressId?: string
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour. */
  showTerminalHintFor?: string
  /** The dock overlays the transcript by padding, so its height changes reach
   *  the anchor only through this. */
  dockHeight?: number
  /** The surface around the list is still taking its final shape. */
  holdReveal?: boolean
  /** A background tab stays mounted behind visibility:hidden. */
  visible?: boolean
  initialScrollPosition?: TranscriptScrollPosition | null
  onScrollPositionChange?: (position: TranscriptScrollPosition) => void
  /** The overlay header's full dissolve reach, for the anchor's pin math. */
  headerClearancePx?: number
}

/** `.stream`'s 18px gap, baked into each virtual row so the offsets count it. */
const ROW_GAP = 18

/** Whole pixels: `resizeItem` treats any float drift as a real resize. */
function measureRowHeight(el: Element): number {
  return Math.round(layoutHeight(el))
}

/** A `display:none` workspace reports 0 for every row; the cached height is the
 *  honest answer, else the total collapses and the restored offset clamps. */
function measureRowHeightOrCached(
  el: HTMLDivElement,
  _entry: ResizeObserverEntry | undefined,
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
): number {
  const height = measureRowHeight(el)
  if (height > 0) return height
  const key = instance.options.getItemKey(instance.indexFromElement(el))
  return instance.itemSizeCache.get(key) ?? height
}

/** Frames the total height must hold still before an opening chat is shown:
 *  the measure → scroll → re-range cascade plateaus for a frame mid-way. */
const SETTLE_QUIET_FRAMES = 3

const ESTIMATED_ROW_HEIGHT = 96
const ROW_CHROME = 8 + 30 + ROW_GAP
const ROW_LINE_HEIGHT = 23
const CHARS_PER_LINE = 88

/** Scaled by the text so a long reply's first guess is close to its height. */
function estimateRowHeight(item: TranscriptItem | undefined): number {
  if (item?.kind !== 'text' && item?.kind !== 'user') return ESTIMATED_ROW_HEIGHT
  const lines = Math.max(1, Math.ceil(item.text.length / CHARS_PER_LINE))
  return Math.max(ESTIMATED_ROW_HEIGHT, ROW_CHROME + lines * ROW_LINE_HEIGHT)
}

/** Tool, ask and marker rows of one run sit flush; prose gets the gap. */
const compact = (item: TranscriptItem | undefined) =>
  item?.kind === 'tool' || item?.kind === 'ask' || item?.kind === 'marker'

/** tanstack's `observeElementRect`, held back while a pane is dragged and never
 *  reporting a parked (0×0) view, which would unmount every row. */
function observeScrollRect(
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
  cb: (rect: { width: number; height: number }) => void,
) {
  const element = instance.scrollElement
  if (!element) return
  const report = (rect: { width: number; height: number }) => {
    if (rect.width === 0 && rect.height === 0) return
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

const samePlace = (a: RowPlace | undefined, b: RowPlace) =>
  !!a &&
  a.firstTurn === b.firstTurn &&
  a.firstReply === b.firstReply &&
  a.turnbar === b.turnbar &&
  a.precedingUserAt === b.precedingUserAt

/** What each row's position says: the first turn and its reply keep the
 *  document hand, a reply is timed from its prompt, and only the reply that
 *  hands control back (no reply after it before the next prompt) gets a turnbar. */
function placesOf(
  ids: string[],
  items: Record<string, TranscriptItem>,
  working: boolean,
): Map<string, RowPlace> {
  const places = new Map<string, RowPlace>()
  const first = items[ids[0] ?? '']
  const firstTurn = first?.kind === 'user' ? first.id : undefined
  let firstReply: string | undefined
  let lastUserAt: string | undefined
  let lastText: string | undefined
  const closeRun = () => {
    const place = lastText ? places.get(lastText) : undefined
    if (place && !working) place.turnbar = true
    lastText = undefined
  }
  for (const id of ids) {
    const item = items[id]
    if (item?.kind === 'user') {
      if (item.origin === 'harness' || item.origin === 'note') continue
      closeRun()
      lastUserAt = item.at
      places.set(id, { firstTurn: id === firstTurn })
    } else if (item?.kind === 'text') {
      if (firstTurn && !firstReply) firstReply = id
      lastText = id
      places.set(id, { firstReply: id === firstReply, turnbar: false, precedingUserAt: lastUserAt })
    }
  }
  closeRun()
  return places
}

/**
 * The conversation, bottom-anchored. Rows come from the chat's feed by id;
 * queued prompts sit below the record and above the working line, the order
 * they will happen in.
 */
// react-doctor-disable-next-line no-giant-component -- the virtualizer, anchor and settle gate share refs; T6 owns splitting scroll out
export function AgentTranscript(props: AgentTranscriptProps) {
  const { chatId, queue, dockHeight } = props
  const order = useChatOrder(chatId)
  const liveTail = useLiveTail(chatId)
  const items = useChatItems(chatId)
  const [settled, setSettled] = useState(false)
  const anchor = useTranscriptAnchor({
    loadingHistory: props.status === 'connecting',
    initialPosition: props.initialScrollPosition,
    onPositionChange: props.onScrollPositionChange,
    visible: props.visible,
    headerClearancePx: props.headerClearancePx,
  })
  const scrollFrame = useScrollFrameSpan()
  const { notifyReflow, pinTurnToTop, scrollRef } = anchor
  useEffect(() => {
    notifyReflow()
  }, [dockHeight, notifyReflow])

  const rows = useMemo(() => {
    const all = liveTail.length > 0 ? [...order, ...liveTail] : order
    return props.suppressId ? all.filter((id) => id !== props.suppressId) : all
  }, [order, liveTail, props.suppressId])

  // Place objects keep their identity while unchanged, so memoized rows skip.
  const placeCache = useRef(new Map<string, RowPlace>())
  const places = useMemo(() => {
    const next = placesOf(rows, items ?? {}, props.working)
    for (const [id, place] of next) {
      const prev = placeCache.current.get(id)
      if (samePlace(prev, place)) next.set(id, prev!)
    }
    placeCache.current = next
    return next
  }, [rows, items, props.working])

  // A turn starting lifts the prompt that started it to the top, so its reply
  // has the viewport to grow into. A prompt queued behind a running turn starts
  // nothing, and one inherited on reopen is a restore, not a send.
  const pinnedRequestId = useRef<string | null>(null)
  const pinnedItem = useRef<PromptQueueItem | null>(null)
  const sawFirstQueue = useRef(false)
  const onQueueChange = useEffectEvent(() => {
    const newestItem = queue.at(-1) ?? null
    const newest = newestItem?.clientRequestId ?? null
    const inherited = !sawFirstQueue.current
    sawFirstQueue.current = true
    if (newest === pinnedRequestId.current) return
    pinnedRequestId.current = newest
    if (inherited) return
    const root = scrollRef.current
    if (newest) {
      pinnedItem.current = newestItem
      if (props.working) return
      const row = root?.querySelector<HTMLElement>(
        `[data-client-request-id="${CSS.escape(newest)}"]`,
      )
      if (row) pinTurnToTop(row)
      return
    }
    // The queue drained: re-anchor on the prompt's own row, or release a pin
    // whose prompt was cancelled before it sent (nothing will fill its room).
    const id = pinnedItem.current ? `p:${pinnedItem.current.clientRequestId}` : null
    const row = id ? root?.querySelector<HTMLElement>(`[data-row-id="${CSS.escape(id)}"]`) : null
    pinTurnToTop(row ?? null)
    pinnedItem.current = null
  })
  useLayoutEffect(() => onQueueChange(), [queue])

  const getItemKey = useCallback((index: number) => rows[index] ?? index, [rows])
  const rowVirtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => estimateRowHeight(items?.[rows[index] ?? '']),
    overscan: 12,
    measureElement: measureRowHeightOrCached,
    getItemKey,
    observeElementRect: observeScrollRect,
    // `measureElement` runs in React's commit; flushSync from there throws.
    useFlushSync: false,
  })

  // Shows the list once its total height stops moving (see SETTLE_QUIET_FRAMES).
  // A hidden tab or an unlaid-out pane is not converging yet, so it never latches.
  useEffect(() => {
    if (settled || rows.length === 0 || props.visible === false || props.holdReveal) return
    let quiet = 0
    let last = -1
    let frame = 0
    const step = () => {
      const el = scrollRef.current
      if (!el || el.clientHeight === 0 || rowVirtualizer.getVirtualItems().length === 0) {
        quiet = 0
        last = -1
        frame = requestAnimationFrame(step)
        return
      }
      const total = rowVirtualizer.getTotalSize() + (dockHeight ?? 0)
      quiet = total === last ? quiet + 1 : 0
      last = total
      if (quiet >= SETTLE_QUIET_FRAMES) {
        setSettled(true)
        return
      }
      frame = requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
    return () => cancelAnimationFrame(frame)
  }, [settled, rows.length, rowVirtualizer, props.visible, props.holdReveal, dockHeight, scrollRef])

  useEffect(() => {
    if (!settled) return
    const frame = requestAnimationFrame(() => markEnd('chat:first-row'))
    return () => cancelAnimationFrame(frame)
  }, [settled])

  // A queued row's last real height primes its record row, so the swap from one
  // to the other never drops the total for a frame.
  const lastQueuedHeight = useRef(new Map<string, number>())
  useLayoutEffect(() => {
    const container = scrollRef.current
    if (!container) return
    for (const item of queue) {
      const el = container.querySelector<HTMLElement>(
        `[data-client-request-id="${CSS.escape(item.clientRequestId)}"]`,
      )
      if (el) lastQueuedHeight.current.set(`p:${item.clientRequestId}`, measureRowHeight(el))
    }
  }, [queue, scrollRef])
  useLayoutEffect(() => {
    if (lastQueuedHeight.current.size === 0) return
    rows.forEach((id, index) => {
      const height = lastQueuedHeight.current.get(id)
      if (height === undefined) return
      lastQueuedHeight.current.delete(id)
      rowVirtualizer.resizeItem(index, height)
    })
  }, [rows, rowVirtualizer])

  const empty = rows.length === 0
  return (
    <div
      className="scroll"
      data-testid="agent-message-list"
      ref={scrollRef}
      onScroll={() => {
        anchor.onScroll()
        scrollFrame.onScrollEvent()
      }}
      // A real gesture outranks the opening gate; onScroll also fires for the
      // anchor's own writes, so it cannot release it.
      onWheel={() => setSettled(true)}
      onPointerDown={() => setSettled(true)}
    >
      <div className="scroll-spacer" aria-hidden="true" />
      <div className="center stream">
        {props.status === 'connecting' && empty && queue.length === 0 && (
          <div className="flex flex-1 items-center justify-center gap-2 text-muted-foreground text-sm">
            <FlickerSpinner className="size-4" /> Loading messages…
          </div>
        )}
        {props.status === 'error' && empty && (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
            <p>Couldn’t reach this chat. Crowbar keeps trying.</p>
            <Button size="sm" variant="ghost" onClick={props.onOpenTerminal}>
              <TerminalIcon /> Terminal
            </Button>
          </div>
        )}
        {!empty && (
          <div
            className="virtual-rows"
            style={{
              height: `${rowVirtualizer.getTotalSize()}px`,
              ...(settled ? null : { visibility: 'hidden' }),
            }}
          >
            {rowVirtualizer.getVirtualItems().map((virtualRow) => {
              const id = rows[virtualRow.index]
              if (!id) return null
              const next = rows[virtualRow.index + 1]
              const flush = !next || (compact(items?.[id]) && compact(items?.[next]))
              const firstTurn = places.get(id)?.firstTurn === true
              return (
                <div
                  key={id}
                  ref={rowVirtualizer.measureElement}
                  data-index={virtualRow.index}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                    paddingBottom: flush || firstTurn ? 0 : ROW_GAP,
                  }}
                >
                  <TranscriptRow
                    chatId={chatId}
                    id={id}
                    providers={props.providers}
                    place={places.get(id)}
                  />
                  {firstTurn && <FirstTurnDivider />}
                </div>
              )
            })}
          </div>
        )}
        {queue.map((item, index) => {
          const isFirstTurn = index === 0 && empty
          // A Fragment: `.queued` aligns as a direct flex child of `.stream`.
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
          working={props.working}
          since={props.since}
          compacting={props.compacting}
          reasoning={props.reasoning}
          plan={props.plan}
          asking={props.asking}
        />
        <div className="dock-spacer" aria-hidden="true" />
      </div>
    </div>
  )
}

import {
  type ReactNode,
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { flushSync } from 'react-dom'
import {
  elementScroll,
  observeElementOffset,
  useVirtualizer,
  type VirtualItem,
  type Virtualizer,
} from '@tanstack/react-virtual'
import { layoutHeight } from '@/lib/layout-size'
import { IS_MOBILE } from '@/utils/platform'
import { markEnd } from '@/lib/perf/instrumentation'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import {
  useChatItems,
  useDisplayRows,
  useFeedError,
  useLiveTail,
} from '@/features/agent/hooks/use-chat-feed'
import {
  useTranscriptAnchor,
  type TranscriptScrollPosition,
} from '@/features/agent/hooks/use-transcript-anchor'
import { useOlderTurns } from '@/features/agent/hooks/use-older-turns'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import type {
  ChatFeed,
  TranscriptItem,
  UserItem,
} from '@/features/agent/stores/lib/transcript-items'
import { FirstTurnDivider } from '@/features/agent/transcript/first-turn-divider'
import { JumpToLatest } from '@/features/agent/transcript/jump-to-latest'
import { copyWithoutAnchors } from '@/features/agent/transcript/lib/plain-copy'
import {
  TranscriptRow,
  type RowActions,
  type RowPlace,
} from '@/features/agent/transcript/rows/transcript-row'

interface AgentTranscriptProps {
  chatId: string
  providers: AgentProvider[]
  status?: ChatFeed['status']
  working: boolean
  /** Drawn after the rows: the chat's working line. */
  tail?: ReactNode
  onOpenTerminal: () => void
  /** Stable for the view's life: every prompt row holds it. */
  promptActions: RowActions
  /** The dock overlays the transcript by padding, so its height changes reach
   *  the anchor only through this. */
  dockHeight?: number
  /** A background tab stays mounted behind visibility:hidden. */
  visible?: boolean
  initialScrollPosition?: TranscriptScrollPosition | null
  onScrollPositionChange?: (position: TranscriptScrollPosition) => void
  /** The overlay header's full dissolve reach, for the anchor's pin math. */
  headerClearancePx?: number
}

type Key = VirtualItem['key']

/** `.stream`'s 18px gap, baked into each virtual row so the offsets count it. */
const ROW_GAP = 18

/** `.dock-spacer`'s height past the dock's own (transcript-layout.css). */
const DOCK_ROOM = 32

/** Whole pixels: `resizeItem` treats any float drift as a real resize. */
function measureRowHeight(el: Element): number {
  return Math.round(layoutHeight(el))
}

/** A `display:none` workspace reports 0 for every row; the cached height is the
 *  honest answer, else the total collapses and the restored offset clamps.
 *  `measured` records which rows have a real height, not an estimate. */
const measureRowInto =
  (measured: Set<Key>) =>
  (
    el: HTMLDivElement,
    _entry: ResizeObserverEntry | undefined,
    instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
  ): number => {
    const height = measureRowHeight(el)
    const key = instance.options.getItemKey(instance.indexFromElement(el))
    if (height > 0) {
      measured.add(key)
      return height
    }
    return instance.itemSizeCache.get(key) ?? height
  }

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
const compact = (id: string, item: TranscriptItem | undefined) =>
  id.startsWith('g:') || item?.kind === 'tool' || item?.kind === 'ask' || item?.kind === 'marker'

/** tanstack's `observeElementRect`, held back while a pane is dragged and never
 *  reporting a parked (0×0) view, which would unmount every row. The anchor hears
 *  every report, parked ones too: it lands the view when the box comes back. */
const observeScrollRectFor =
  (onReport: () => void) =>
  (
    instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
    cb: (rect: { width: number; height: number }) => void,
  ) => {
    const element = instance.scrollElement
    if (!element) return
    const report = (rect: { width: number; height: number }) => {
      if (rect.width > 0 || rect.height > 0) {
        cb({ width: Math.round(rect.width), height: Math.round(rect.height) })
      }
      onReport()
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
      // Between layout and paint: the window for the new box renders before it shows.
      // WebKit may log 'ResizeObserver loop completed with undelivered notifications': benign.
      flushSync(() => report(rect))
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

type OffsetReport = (offset: number) => void

/** At a held bottom the anchor answers a resize (the virtualizer's shift paints first).
 *  Elsewhere: virtual-core 3.17.11's default, which setting this predicate replaces;
 *  pinned by the test 'leaves a reader be while the row they are reading grows below the top'. */
const keepsRowsStill =
  (holdsBottom: () => boolean) =>
  (item: VirtualItem, _delta: number, instance: Virtualizer<HTMLDivElement, HTMLDivElement>) => {
    if (holdsBottom()) return false
    const offset = (instance.scrollOffset ?? 0) + instance.scrollAdjustments
    if (!instance.itemSizeCache.has(item.key)) return item.start < offset
    return item.end <= offset && instance.scrollDirection !== 'backward'
  }

/** tanstack's `observeElementOffset`, also handed every offset the anchor writes
 *  (through `hear`): its scroll event alone arrives a frame late, after the old
 *  window is painted at the new offset. */
const observeScrollOffsetFor =
  (hear: (report: OffsetReport | null) => void) =>
  (
    instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
    cb: (offset: number, isScrolling: boolean) => void,
  ) => {
    if (!instance.scrollElement) return
    hear((offset) => cb(offset, false))
    const stop = observeElementOffset(instance, cb)
    return () => {
      hear(null)
      stop?.()
    }
  }

/** The pinned prompt sits at the top when `.stream` reaches one viewport, less
 *  the overlay header, below the row's start; the floor counts everything under
 *  the rows (the working line, the dock's room) without measuring it. */
const pinFloor = (start: number, viewport: number, header: number) =>
  Math.max(0, start + viewport - header)

const samePlace = (a: RowPlace | undefined, b: RowPlace) =>
  !!a &&
  a.firstTurn === b.firstTurn &&
  a.firstReply === b.firstReply &&
  a.turnbar === b.turnbar &&
  a.precedingUserAt === b.precedingUserAt

/** Each row's place: the first turn and reply keep the document hand, a reply is timed from its
 *  prompt, and only the reply that hands control back (none after it before the next prompt) gets
 *  a turnbar; the newest run hands it back only once nothing runs. */
function placesOf(
  ids: string[],
  itemOf: (id: string) => TranscriptItem | undefined,
  working: boolean,
  fromStart: boolean,
): Map<string, RowPlace> {
  const places = new Map<string, RowPlace>()
  // A window of a longer chat starts mid-way: its first prompt is not the chat's.
  const first = fromStart ? itemOf(ids[0] ?? '') : undefined
  const firstTurn = first?.kind === 'user' ? first.id : undefined
  let firstReply: string | undefined
  let lastUserAt: string | undefined
  let lastText: string | undefined
  const closeRun = (handedBack: boolean) => {
    const place = lastText ? places.get(lastText) : undefined
    if (place && handedBack) place.turnbar = true
    lastText = undefined
  }
  for (const id of ids) {
    const item = itemOf(id)
    if (item?.kind === 'user') {
      if (item.origin === 'harness' || item.origin === 'note') continue
      closeRun(true)
      lastUserAt = item.at
      places.set(id, { firstTurn: id === firstTurn })
    } else if (item?.kind === 'text') {
      if (firstTurn && !firstReply) firstReply = id
      lastText = id
      places.set(id, { firstReply: id === firstReply, turnbar: false, precedingUserAt: lastUserAt })
    } else if (item?.kind !== 'marker') {
      // A call (or run of them) after the words: those words did not hand back.
      lastText = undefined
    }
  }
  closeRun(!working)
  return places
}

const NO_OUTBOX: OutboxItem[] = []

/** An outbox prompt as the row layout reads it: a user turn with no row yet. */
const asUser = (item: OutboxItem): UserItem => ({
  kind: 'user',
  id: `p:${item.requestId}`,
  seq: 0,
  at: '',
  text: item.text,
  origin: 'crowbar',
  requestId: item.requestId,
})

/** The conversation, bottom-anchored. Rows come from the chat's feed by id;
 *  prompts with no row yet follow them, in the order they will be sent. */
// react-doctor-disable-next-line no-giant-component -- the virtualizer, anchor and pin share the row offsets
export function AgentTranscript(props: AgentTranscriptProps) {
  const { chatId } = props
  const display = useDisplayRows(chatId)
  const liveTail = useLiveTail(chatId)
  const refusal = useFeedError(chatId)
  const items = useChatItems(chatId)
  const outbox = useChatStore((s) => s.outbox[chatId] ?? NO_OUTBOX)
  const [measured] = useState(() => new Set<Key>())
  const [pinnedId, setPinnedId] = useState<string | null>(null)
  // The pin's turn is over once its prompt left `pending` and nothing runs.
  const pinnedState = useChatStore((s) =>
    pinnedId ? s.feeds[chatId]?.facts.prompts[pinnedId.slice(2)]?.state : undefined,
  )
  const turnOver = !props.working && pinnedState !== undefined && pinnedState !== 'pending'
  // Released once, for a reader at the bottom: one reading up the page keeps the
  // room (and their place) until they come back down.
  const [releasedId, setReleasedId] = useState<string | null>(null)
  const reportOffset = useRef<OffsetReport | null>(null)
  const anchor = useTranscriptAnchor({
    loadingHistory: props.status === 'connecting',
    initialPosition: props.initialScrollPosition,
    onPositionChange: props.onScrollPositionChange,
    visible: props.visible,
    arriving: props.working || (pinnedId !== null && !turnOver),
    onWrite: (top) => reportOffset.current?.(top),
  })
  const { afterCommit, notifyReflow, noteScrollWrite, stick, holdsBottom, scrollRef } = anchor
  const holding = useSyncExternalStore(anchor.subscribe, holdsBottom)
  const releaseFor = turnOver ? (holding ? pinnedId : releasedId) : null
  if (releaseFor !== releasedId) setReleasedId(releaseFor)
  const released = turnOver && releaseFor === pinnedId
  const [measureRow] = useState(() => measureRowInto(measured))
  const [observeElementRect] = useState(() => observeScrollRectFor(notifyReflow))
  const [observeOffset] = useState(() =>
    observeScrollOffsetFor((report) => {
      reportOffset.current = report
    }),
  )

  // An outbox prompt whose row landed is the rows' from then on, under the same key.
  const unsent = useMemo(() => {
    const byId = new Map<string, UserItem>()
    for (const item of outbox) {
      const id = `p:${item.requestId}`
      if (!items?.has(id)) byId.set(id, asUser(item))
    }
    return byId
  }, [outbox, items])
  const order = useMemo(() => display.map((row) => row.id), [display])
  const runs = useMemo(
    () => new Map(display.flatMap((row) => (row.members ? [[row.id, row.members]] : []))),
    [display],
  )
  const rows = useMemo(() => {
    return liveTail.length + unsent.size > 0 ? [...order, ...liveTail, ...unsent.keys()] : order
  }, [order, liveTail, unsent])
  const itemOf = useCallback((id: string) => items?.get(id) ?? unsent.get(id), [items, unsent])
  const fromStart = useChatStore((s) => !s.feeds[chatId]?.from)

  // Place objects keep their identity while unchanged, so memoized rows skip.
  const placeCache = useRef(new Map<string, RowPlace>())
  const places = useMemo(() => {
    const next = placesOf(rows, itemOf, props.working, fromStart)
    for (const [id, place] of next) {
      const prev = placeCache.current.get(id)
      if (samePlace(prev, place)) next.set(id, prev!)
    }
    placeCache.current = next
    return next
  }, [rows, itemOf, props.working, fromStart])

  // A send lifts its prompt to the top, so its reply has the viewport to grow
  // into. A prompt still queued (behind a turn, or held) starts nothing, and one
  // already waiting on reopen is a restore, not a send.
  const newestUnsent = outbox.at(-1)?.requestId ?? null
  const newestSending = outbox.at(-1)?.state === 'sending'
  const pinnedRequestId = useRef<string | null>(null)
  const pendingPin = useRef<string | null>(null)
  const sawFirstOutbox = useRef(false)
  const pin = (id: string) => {
    // The phone has no room to spare for a reply that has not arrived yet.
    if (IS_MOBILE || id === pinnedId) return
    stick()
    setPinnedId(id)
  }
  const onOutboxChange = useEffectEvent(() => {
    const inherited = !sawFirstOutbox.current
    sawFirstOutbox.current = true
    if (newestUnsent !== pinnedRequestId.current) {
      pinnedRequestId.current = newestUnsent
      if (inherited) return
      if (!newestUnsent) {
        // The outbox drained: pin the prompt that was waiting once its row is in. One
        // taken back has no row, so its pin has no offset and reserves nothing.
        const waiting = pendingPin.current
        pendingPin.current = null
        if (waiting && items?.has(waiting)) pin(waiting)
        return
      }
      pendingPin.current = `p:${newestUnsent}`
    }
    if (newestSending && !props.working && pendingPin.current === `p:${newestUnsent}`) {
      pin(pendingPin.current)
    }
  })
  useLayoutEffect(() => onOutboxChange(), [newestUnsent, newestSending])

  const getItemKey = useCallback((index: number) => rows[index] ?? index, [rows])
  const rowVirtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => estimateRowHeight(itemOf(rows[index] ?? '')),
    overscan: 12,
    measureElement: measureRow,
    getItemKey,
    observeElementRect,
    observeElementOffset: observeOffset,
    // Its writes keep a reader's rows still as rows above them change size.
    scrollToFn: (offset, options, instance) => {
      elementScroll(offset, options, instance)
      noteScrollWrite()
    },
    // `measureElement` runs in React's commit; flushSync from there throws.
    useFlushSync: false,
  })
  useLayoutEffect(() => {
    rowVirtualizer.shouldAdjustScrollPositionOnItemSizeChange = keepsRowsStill(holdsBottom)
  }, [rowVirtualizer, holdsBottom])
  const virtualItems = rowVirtualizer.getVirtualItems()
  const measuring = virtualItems.some((row) => !measured.has(row.key))
  useOlderTurns(chatId, virtualItems[0]?.index, scrollRef, anchor.keepPlace)
  useLayoutEffect(() => afterCommit(measuring))

  const pinIndex = pinnedId ? rows.indexOf(pinnedId) : -1
  const pinStart = pinIndex < 0 ? undefined : rowVirtualizer.measurementsCache[pinIndex]?.start
  // A released pin's room slides (transcript-layout.css) down to the rows' own height,
  // which the floor must reach exactly, else the slide spends itself above or below it.
  const floor =
    pinStart === undefined
      ? 0
      : released
        ? rowVirtualizer.getTotalSize() + ROW_GAP + (props.dockHeight ?? 0) + DOCK_ROOM
        : pinFloor(pinStart, rowVirtualizer.scrollRect?.height ?? 0, props.headerClearancePx ?? 0)

  const empty = rows.length === 0
  useEffect(() => {
    if (empty) return
    const frame = requestAnimationFrame(() => markEnd('chat:first-row'))
    return () => cancelAnimationFrame(frame)
  }, [empty])

  const jumpToLatest = () => {
    stick()
    notifyReflow()
  }

  return (
    <div
      className="scroll"
      data-testid="agent-message-list"
      ref={scrollRef}
      onScroll={anchor.onScroll}
      onCopy={copyWithoutAnchors}
      {...anchor.intent}
    >
      <div className="scroll-spacer" aria-hidden="true" />
      <div
        className="center stream"
        style={floor > 0 ? { minHeight: floor } : undefined}
        data-pin={released ? 'released' : undefined}
      >
        {props.status === 'connecting' && empty && (
          <div className="flex flex-1 items-center justify-center gap-2 text-muted-foreground text-sm">
            <FlickerSpinner className="size-4" /> Loading messages…
          </div>
        )}
        {props.status === 'error' && empty && (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
            <p>{refusal || 'Couldn’t reach this chat. Crowbar keeps trying.'}</p>
            <Button size="sm" variant="ghost" onClick={props.onOpenTerminal}>
              <TerminalIcon /> Terminal
            </Button>
          </div>
        )}
        {!empty && (
          <div className="virtual-rows" style={{ height: `${rowVirtualizer.getTotalSize()}px` }}>
            {virtualItems.map((virtualRow) => {
              const id = rows[virtualRow.index]
              if (!id) return null
              const next = rows[virtualRow.index + 1]
              const flush = !next || (compact(id, itemOf(id)) && compact(next, itemOf(next)))
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
                    actions={props.promptActions}
                    members={runs.get(id)}
                  />
                  {firstTurn && <FirstTurnDivider />}
                </div>
              )
            })}
          </div>
        )}
        {props.tail}
        {!holding && !empty && <JumpToLatest onJump={jumpToLatest} />}
        <div className="dock-spacer" aria-hidden="true" />
      </div>
    </div>
  )
}

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
import { isNotice } from '@/features/agent/transcript/rows/marker-row'
import { copyWithoutAnchors } from '@/features/agent/transcript/lib/plain-copy'
import { ROW_GAP, estimateRowHeight, reachRange } from '@/features/agent/transcript/lib/row-reach'
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
  tail?: ReactNode
  onOpenTerminal: () => void
  promptActions: RowActions
  dockHeight?: number
  dockTop?: number
  visible?: boolean
  initialScrollPosition?: TranscriptScrollPosition | null
  onScrollPositionChange?: (position: TranscriptScrollPosition) => void
  headerClearancePx?: number
}

const OVERSCAN = 12
const NOTICE_GAP = 10

const DOCK_ROOM = IS_MOBILE ? 14 : 32

function measureRow(
  el: HTMLDivElement,
  _entry: ResizeObserverEntry | undefined,
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
): number {
  const height = Math.round(layoutHeight(el))
  return height > 0
    ? height
    : (instance.itemSizeCache.get(instance.options.getItemKey(instance.indexFromElement(el))) ??
        height)
}

const compact = (id: string, item: TranscriptItem | undefined) =>
  id.startsWith('g:') ||
  item?.kind === 'tool' ||
  item?.kind === 'ask' ||
  (item?.kind === 'marker' && !isNotice(item))

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

const keepsRowsStill =
  (holdsBottom: () => boolean) =>
  (item: VirtualItem, _delta: number, instance: Virtualizer<HTMLDivElement, HTMLDivElement>) => {
    if (holdsBottom()) return false
    const offset = (instance.scrollOffset ?? 0) + instance.scrollAdjustments
    if (!instance.itemSizeCache.has(item.key)) return item.start < offset
    return item.end <= offset && instance.scrollDirection !== 'backward'
  }

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

const pinFloor = (start: number, viewport: number, header: number) =>
  Math.max(0, start + viewport - header)

const samePlace = (a: RowPlace | undefined, b: RowPlace) =>
  !!a && a.firstTurn === b.firstTurn && a.firstReply === b.firstReply && a.turnbar === b.turnbar

function placesOf(
  ids: string[],
  itemOf: (id: string) => TranscriptItem | undefined,
  working: boolean,
  fromStart: boolean,
): Map<string, RowPlace> {
  const places = new Map<string, RowPlace>()
  const first = fromStart ? itemOf(ids[0] ?? '') : undefined
  const firstTurn = first?.kind === 'user' ? first.id : undefined
  let firstReply: string | undefined
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
      places.set(id, { firstTurn: id === firstTurn })
    } else if (item?.kind === 'text') {
      if (firstTurn && !firstReply) firstReply = id
      lastText = id
      places.set(id, { firstReply: id === firstReply, turnbar: false })
    } else if (item?.kind !== 'marker') {
      lastText = undefined
    }
  }
  closeRun(!working)
  return places
}

const NO_OUTBOX: OutboxItem[] = []

const asUser = (item: OutboxItem): UserItem => ({
  kind: 'user',
  id: `p:${item.requestId}`,
  seq: 0,
  at: '',
  text: item.text,
  origin: 'crowbar',
  requestId: item.requestId,
})

// react-doctor-disable-next-line no-giant-component -- the virtualizer, anchor and pin share the row offsets
export function AgentTranscript(props: AgentTranscriptProps) {
  const { chatId } = props
  const display = useDisplayRows(chatId)
  const liveTail = useLiveTail(chatId)
  const refusal = useFeedError(chatId)
  const items = useChatItems(chatId)
  const outbox = useChatStore((s) => s.outbox[chatId] ?? NO_OUTBOX)
  const [pinnedId, setPinnedId] = useState<string | null>(null)
  const pinnedState = useChatStore((s) =>
    pinnedId ? s.feeds[chatId]?.facts.prompts[pinnedId.slice(2)]?.state : undefined,
  )
  const turnOver = !props.working && pinnedState !== undefined && pinnedState !== 'pending'
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
  const away = useSyncExternalStore(anchor.subscribe, anchor.awayFromEnd)
  const releaseFor = turnOver ? (holding ? pinnedId : releasedId) : null
  if (releaseFor !== releasedId) setReleasedId(releaseFor)
  const released = turnOver && releaseFor === pinnedId
  const [observeElementRect] = useState(() => observeScrollRectFor(notifyReflow))
  const [observeOffset] = useState(() =>
    observeScrollOffsetFor((report) => {
      reportOffset.current = report
    }),
  )

  const unsent = useMemo(() => {
    const byId = new Map<string, UserItem>()
    for (const item of outbox) {
      if (item.state !== 'sending') continue
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

  const newestUnsent = outbox.at(-1)?.requestId ?? null
  const newestSending = outbox.at(-1)?.state === 'sending'
  const pinnedRequestId = useRef<string | null>(null)
  const pendingPin = useRef<string | null>(null)
  const sawFirstOutbox = useRef(false)
  const pin = (id: string) => {
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
  const sizeOf = (index: number) => {
    const id = rows[index] ?? ''
    return (
      rowVirtualizer.itemSizeCache.get(id) ??
      estimateRowHeight(id, itemOf(id), rowVirtualizer.scrollRect?.width)
    )
  }
  const rangeExtractor = reachRange(sizeOf, OVERSCAN)
  const rowVirtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: sizeOf,
    overscan: OVERSCAN,
    rangeExtractor,
    measureElement: measureRow,
    getItemKey,
    observeElementRect,
    observeElementOffset: observeOffset,
    scrollToFn: (offset, options, instance) => {
      elementScroll(offset, options, instance)
      noteScrollWrite()
    },
    useFlushSync: false,
  })
  useLayoutEffect(() => {
    rowVirtualizer.shouldAdjustScrollPositionOnItemSizeChange = keepsRowsStill(holdsBottom)
  }, [rowVirtualizer, holdsBottom])
  const virtualItems = rowVirtualizer.getVirtualItems()
  const measuring = virtualItems.some((row) => !rowVirtualizer.itemSizeCache.has(row.key))
  useOlderTurns(chatId, scrollRef, anchor.keepPlace)
  useLayoutEffect(() => afterCommit(measuring))

  const pinIndex = pinnedId ? rows.indexOf(pinnedId) : -1
  const pinStart = pinIndex < 0 ? undefined : rowVirtualizer.measurementsCache[pinIndex]?.start
  const floor =
    pinStart === undefined
      ? 0
      : released
        ? rowVirtualizer.getTotalSize() + ROW_GAP + (props.dockHeight ?? 0) + DOCK_ROOM
        : pinFloor(pinStart, rowVirtualizer.scrollRect?.height ?? 0, props.headerClearancePx ?? 0)

  const empty = rows.length === 0
  useEffect(() => {
    if (empty) return
    const frame = requestAnimationFrame(() => {
      markEnd('chat:first-row')
    })
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
              const noticeRun = !!next && isNotice(itemOf(id)) && isNotice(itemOf(next))
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
                    paddingBottom: flush || firstTurn ? 0 : noticeRun ? NOTICE_GAP : ROW_GAP,
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
        {away && !empty && props.visible !== false && (
          <JumpToLatest onJump={jumpToLatest} dockTop={props.dockTop || props.dockHeight || 92} />
        )}
        <div className="dock-spacer" aria-hidden="true" />
      </div>
    </div>
  )
}

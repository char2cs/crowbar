import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { createFollowScroll, type FollowScroll } from '@/features/agent/hooks/lib/follow-scroll'

const STICK_SLACK = 96

const AWAY_VIEWPORTS = 1 / 3

export interface TranscriptScrollPosition {
  stuck: boolean
  distanceFromBottom: number
}

interface UseTranscriptAnchorOptions {
  loadingHistory?: boolean
  initialPosition?: TranscriptScrollPosition | null
  onPositionChange?: (position: TranscriptScrollPosition) => void
  visible?: boolean
  arriving?: boolean
  onWrite?: (top: number) => void
}

export interface TranscriptAnchor {
  scrollRef: React.RefObject<HTMLDivElement | null>
  onScroll: () => void
  afterCommit: (measuring: boolean) => void
  notifyReflow: () => void
  noteScrollWrite: () => void
  stick: () => void
  keepPlace: () => void
  intent: ReaderIntent
  holdsBottom: () => boolean
  awayFromEnd: () => boolean
  subscribe: (listener: () => void) => () => void
}

interface ReaderIntent {
  onWheel: (event: React.WheelEvent) => void
  onKeyDown: (event: React.KeyboardEvent) => void
  onTouchStart: (event: React.TouchEvent) => void
  onTouchMove: (event: React.TouchEvent) => void
}

const READ_UP_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

const WHEEL_JITTER_PX = 4

function takenInside(target: EventTarget, scroller: HTMLElement | null): boolean {
  for (let el = target instanceof Element ? target : null; el && el !== scroller;) {
    const scrolls =
      el.scrollHeight > el.clientHeight && /auto|scroll/.test(getComputedStyle(el).overflowY)
    if (scrolls && el.scrollTop > 0) return true
    el = el.parentElement
  }
  return false
}

const typing = (target: EventTarget) =>
  target instanceof HTMLElement &&
  (target.isContentEditable || target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')

const readsUp = (event: React.KeyboardEvent) =>
  READ_UP_KEYS.has(event.key) || (event.key === ' ' && event.shiftKey)

export function useTranscriptAnchor(options: UseTranscriptAnchorOptions = {}): TranscriptAnchor {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const stuck = useRef(options.initialPosition?.stuck ?? true)
  const fromBottom = useRef<number | null>(options.initialPosition?.distanceFromBottom ?? null)
  const follow = useRef<FollowScroll | null>(null)
  const written = useRef<number | null>(null)
  const easeFrom = useRef<number | null>(null)
  const landNext = useRef(true)
  const bottom = useRef<number | null>(null)
  const holding = useRef(false)
  const reading = useRef(false)
  const seen = useRef<number | null>(null)
  const touchY = useRef<number | null>(null)
  const wasMeasuring = useRef(false)
  const shown = useRef(options.visible ?? true)
  const away = useRef(false)
  const [listeners] = useState(() => new Set<() => void>())
  const judgeAway = useCallback(() => {
    const el = scrollRef.current
    if (!el || el.clientHeight === 0) return
    const room = el.scrollHeight - el.scrollTop - el.clientHeight
    const next = !stuck.current && room > el.clientHeight * AWAY_VIEWPORTS
    if (next === away.current || (next && wasMeasuring.current)) return
    away.current = next
    for (const listener of listeners) listener()
  }, [listeners])
  const setStuck = useCallback(
    (next: boolean) => {
      if (stuck.current === next) return
      stuck.current = next
      for (const listener of listeners) listener()
      judgeAway()
    },
    [listeners, judgeAway],
  )
  const optionsRef = useRef(options)
  optionsRef.current = options

  const wrote = useCallback((top: number) => {
    written.current = top
    seen.current = top
    optionsRef.current.onWrite?.(top)
  }, [])

  const write = useCallback(
    (el: HTMLElement, top: number) => {
      el.scrollTop = top
      wrote(el.scrollTop)
    },
    [wrote],
  )

  const resync = useCallback(
    (correcting = false) => {
      const el = scrollRef.current
      if (!el) return
      if (el.clientHeight === 0) return
      const land = landNext.current
      landNext.current = false
      holding.current = false
      if (!stuck.current) {
        const keep = land ? fromBottom.current : null
        if (keep === null || keep <= el.scrollHeight) {
          if (keep !== null) write(el, Math.max(0, el.scrollHeight - keep))
          fromBottom.current = el.scrollHeight - el.scrollTop
          return
        }
        if (wasMeasuring.current) {
          landNext.current = holding.current = true
          return
        }
        setStuck(true)
      }
      const target = el.scrollHeight - el.clientHeight
      const moved = target !== bottom.current
      bottom.current = target
      if (!moved && !land) return
      const gap = target - el.scrollTop
      if (gap <= 1) easeFrom.current = null
      const from = Math.min(easeFrom.current ?? el.scrollTop, el.scrollTop)
      const reposition =
        land ||
        optionsRef.current.loadingHistory === true ||
        optionsRef.current.arriving !== true ||
        target - from > el.clientHeight ||
        (correcting && easeFrom.current === null)
      if (reposition) {
        if (el.scrollTop < target) write(el, target)
        easeFrom.current = null
      } else if (gap > 1) {
        easeFrom.current = from
        follow.current?.setTarget(target)
      }
      fromBottom.current = el.scrollHeight - el.scrollTop
    },
    [write, setStuck],
  )

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const build = () => createFollowScroll(el, wrote)
    follow.current = build()
    const onFocus = () => {
      follow.current?.stop()
      follow.current = build()
      bottom.current = null
      resync()
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
      follow.current?.stop()
      follow.current = null
      optionsRef.current.onPositionChange?.({
        stuck: stuck.current,
        distanceFromBottom:
          el.clientHeight > 0 && !holding.current
            ? el.scrollHeight - el.scrollTop
            : (fromBottom.current ?? el.scrollHeight - el.scrollTop),
      })
    }
  }, [resync, wrote])

  const afterCommit = useCallback(
    (measuring: boolean) => {
      const correcting = wasMeasuring.current
      wasMeasuring.current = measuring
      resync(correcting)
      judgeAway()
    },
    [resync, judgeAway],
  )

  useLayoutEffect(() => {
    const visible = options.visible ?? true
    if (visible && !shown.current) {
      landNext.current = true
      resync()
    }
    shown.current = visible
  }, [options.visible, resync])

  const notifyReflow = useCallback(() => {
    landNext.current = true
    resync()
  }, [resync])

  const noteScrollWrite = useCallback(() => {
    written.current = scrollRef.current?.scrollTop ?? null
    seen.current = written.current
  }, [])

  const stick = useCallback(() => {
    setStuck(true)
    reading.current = false
  }, [setStuck])

  const keepPlace = useCallback(() => {
    landNext.current = true
  }, [])

  const holdsBottom = useCallback(() => stuck.current, [])
  const subscribe = useCallback(
    (listener: () => void) => {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    [listeners],
  )

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    if (written.current !== null && Math.abs(el.scrollTop - written.current) <= 1) return
    written.current = null
    const rising = seen.current !== null && el.scrollTop < seen.current - 1
    seen.current = el.scrollTop
    fromBottom.current = el.scrollHeight - el.scrollTop
    if (reading.current && rising) return
    setStuck(el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_SLACK)
    if (stuck.current) reading.current = false
    judgeAway()
  }, [setStuck, judgeAway])

  const letGo = useCallback(() => {
    if (!stuck.current) return
    setStuck(false)
    reading.current = true
    easeFrom.current = null
    follow.current?.pause()
    const el = scrollRef.current
    if (el) fromBottom.current = el.scrollHeight - el.scrollTop
  }, [setStuck])

  const [intent] = useState<ReaderIntent>(() => ({
    onWheel: (event) => {
      const jitter = event.deltaMode === 0 && event.deltaY > -WHEEL_JITTER_PX
      const sideways = Math.abs(event.deltaX) >= Math.abs(event.deltaY)
      if (event.deltaY >= 0 || jitter || sideways || event.ctrlKey) return
      if (!takenInside(event.target, scrollRef.current)) letGo()
    },
    onKeyDown: (event) => {
      if (!readsUp(event) || typing(event.target)) return
      if (!takenInside(event.target, scrollRef.current)) letGo()
    },
    onTouchStart: (event) => {
      touchY.current = event.touches[0]?.clientY ?? null
    },
    onTouchMove: (event) => {
      const y = event.touches[0]?.clientY
      if (y !== undefined && touchY.current !== null && y > touchY.current) letGo()
    },
  }))

  return {
    scrollRef,
    onScroll,
    intent,
    afterCommit,
    notifyReflow,
    noteScrollWrite,
    stick,
    keepPlace,
    holdsBottom,
    awayFromEnd: () => away.current,
    subscribe,
  }
}

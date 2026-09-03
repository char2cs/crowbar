import { useEffect, useRef, useState } from 'react'
import {
  getDefaultChatPresentation,
  useSplitPresentationEnabled,
  type ChatPresentation,
} from '@/features/settings/lib/chat-presentation'

export const SPLIT_MIN_HALF_PX = 340
export const SPLIT_SIDE_BY_SIDE_MIN_PX = 780
export const SPLIT_MIN_STACKED_PX = 160
export const SPLIT_DEFAULT_SIZES: [number, number] = [45, 55]

/**
 * Which surface a chat is shown on, and how the split is laid out.
 *
 * The chosen value is SEEDED from the user's preference and never subscribed to
 * it — a chat already open keeps the surface it is on when the setting changes
 * underneath. What IS subscribed is whether split exists at all, because the
 * switcher has to grow and lose its third segment live.
 */
export function useChatPresentation(
  shownChatId: string,
  splitContainerRef: {
    current: HTMLElement | null
  },
) {
  const [chosen, setChosen] = useState<ChatPresentation>(getDefaultChatPresentation)
  const splitEnabled = useSplitPresentationEnabled()
  // THE SURFACE ACTUALLY SHOWN. Derived rather than corrected, so switching the
  // dev toggle off cannot strand a chat on a surface whose button has just gone:
  // there is no state to write back, and no frame in which the two disagree.
  const presentation: ChatPresentation = chosen === 'split' && !splitEnabled ? 'chat' : chosen
  const splitting = presentation === 'split'

  // Whether the way back to the chat is being OFFERED — what Crowbar shows when
  // it moved somebody to the terminal and cannot move them back without
  // overriding a choice they made themselves.
  const [returnOffered, setReturnOffered] = useState(false)
  // WHICH HALF OF THE SPLIT OWNS THE KEYBOARD. Two live surfaces means two things
  // that can eat a keystroke, and the terminal is the greedy one — xterm
  // re-focuses itself, with retries, whenever it is told it is active. So it is
  // only told that while it actually holds the caret. The composer wins by
  // default: typing a prompt into a terminal by accident is the worse mistake.
  const [splitFocus, setSplitFocus] = useState<'chat' | 'terminal'>('chat')

  // Re-seed PER CHAT, not per mount. This pane is RETAINED across chat selection,
  // so a lazy useState initializer runs once for the pane's entire life: turning
  // the preference off and opening a chat did nothing until a full reload, which
  // reads — fairly — as "the setting is broken".
  const [seededFor, setSeededFor] = useState(shownChatId)
  if (seededFor !== shownChatId) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: React's documented "adjust state when a prop changes" pattern. An effect would paint the previous chat's surface for a frame first, which is the flicker this exists to avoid.
    setSeededFor(shownChatId)
    // Split is an INSTRUMENT the user reached for, not a landing surface, so it
    // is the one thing that survives the re-seed: somebody comparing what Crowbar
    // recorded against what the CLI actually printed is doing it to whichever
    // chat they look at next.
    if (!splitting) setChosen(getDefaultChatPresentation())
    setReturnOffered(false)
    setSplitFocus('chat')
  }

  const [splitSizes, setSplitSizes] = useState<[number, number]>(SPLIT_DEFAULT_SIZES)
  // Side by side, or stacked? The PANE's width decides, not the window's — a
  // narrow pane inside a wide window is exactly the case a viewport media query
  // gets wrong. Only observed while the split is up.
  const [splitStacked, setSplitStacked] = useState(false)

  useEffect(() => {
    const container = splitContainerRef.current
    if (!splitting || !container) return
    const measure = () => {
      // A ZERO WIDTH IS "NOT MEASURED YET", NOT "NARROW". The first read can land
      // before layout — and answering it with `stacked` makes the split flash
      // vertical on the way in, then jump. Side by side is the shape this is FOR,
      // so an unknown width keeps it.
      const width = container.getBoundingClientRect().width
      setSplitStacked(width > 0 && width < SPLIT_SIDE_BY_SIDE_MIN_PX)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    measure()
    return () => observer.disconnect()
  }, [splitting, splitContainerRef])

  return {
    presentation,
    chosen,
    setPresentation: setChosen,
    splitEnabled,
    splitting,
    returnOffered,
    setReturnOffered,
    splitFocus,
    setSplitFocus,
    splitSizes,
    setSplitSizes,
    splitStacked,
  }
}

/**
 * How far xterm's character grid stops short of its own element.
 *
 * A TERMINAL DOES NOT RENDER TO THE EDGE OF ITS OWN BOX. xterm fits
 * `floor(width / cellWidth)` columns and holds back room for a scrollbar, so the
 * canvas is narrower than its container — here by ~16px, a strip that is never
 * drawn to. Anything aligned to the ELEMENT lands on empty space, visibly past
 * where the agent's text stops. This measures the real thing.
 *
 * It cannot be a constant: it moves with the font metrics, the pane width and
 * the scrollbar reservation.
 */
export function useTerminalGridSlack(
  columnRef: { current: HTMLElement | null },
  measuring: boolean,
) {
  const [gridSlack, setGridSlack] = useState(0)
  const ref = useRef(columnRef)
  ref.current = columnRef

  useEffect(() => {
    const column = ref.current.current
    if (!measuring || !column) {
      setGridSlack(0)
      return
    }

    const measure = () => {
      const element = column.querySelector('.xterm')
      const canvas = column.querySelector('.xterm canvas')
      // No canvas yet — the renderer has not drawn. The MutationObserver below
      // calls back the moment it appears; do NOT fall back to 0, which is what
      // pinned the status line to the element's edge instead of the grid's.
      if (!element || !canvas) return
      const slack = element.getBoundingClientRect().right - canvas.getBoundingClientRect().right
      setGridSlack(Math.max(0, Math.round(slack)))
    }

    // Two different events change the answer, and BOTH are needed. xterm builds
    // its canvas asynchronously, after this effect's first frame — measuring only
    // on mount reads a terminal that has not rendered and silently yields 0, and
    // since nothing then resizes, it stays 0 forever. On relayout xterm re-fits to
    // a new column count, measured a frame later because the observer fires when
    // the CONTAINER resizes and xterm re-fits after that.
    let frame = 0
    const remeasure = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }

    const resize = new ResizeObserver(remeasure)
    resize.observe(column)
    const mutation = new MutationObserver(remeasure)
    mutation.observe(column, { childList: true, subtree: true })
    remeasure()

    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [measuring])

  return gridSlack
}

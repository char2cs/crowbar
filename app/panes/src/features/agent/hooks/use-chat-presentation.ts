import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import {
  getDefaultChatPresentation,
  useSplitPresentationEnabled,
  type ChatPresentation,
  type LandingChatPresentation,
} from '@/features/settings/lib/chat-presentation'
import { chatStore } from '@/features/agent/stores/chat-store'
import { IS_MOBILE } from '@/utils/platform'

export const SPLIT_MIN_HALF_PX = 340
const SPLIT_SIDE_BY_SIDE_MIN_PX = 780
export const SPLIT_MIN_STACKED_PX = 160
export const SPLIT_DEFAULT_SIZES: [number, number] = [45, 55]

function useSplitMeasure<T>(
  active: boolean,
  containerRef: { current: HTMLElement | null },
  derive: (width: number, height: number) => T,
  initial: T,
): T {
  const deriveRef = useRef(derive)
  deriveRef.current = derive
  const [value, setValue] = useState<T>(initial)

  useEffect(() => {
    const container = containerRef.current
    if (!active || !container) return
    const measure = () => {
      const el = containerRef.current
      if (!el) return
      const w = el.clientWidth
      const h = el.clientHeight
      setValue(deriveRef.current(Number.isFinite(w) ? w : 0, Number.isFinite(h) ? h : 0))
    }
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    measure()
    return () => observer.disconnect()
  }, [active, containerRef])

  return value
}

function deriveStacked(width: number): boolean {
  return width > 0 && width < SPLIT_SIDE_BY_SIDE_MIN_PX
}

function lastChosen(chatId: string): ChatPresentation | undefined {
  return chatStore.getState().presentations[chatId]
}

function rememberChosen(chatId: string, presentation: ChatPresentation): void {
  chatStore.setState((s) => ({ presentations: { ...s.presentations, [chatId]: presentation } }))
}

export function presetChatLandingPresentation(
  chatId: string,
  presentation: LandingChatPresentation,
): void {
  rememberChosen(chatId, presentation)
}

function shownPresentation(
  chosen: ChatPresentation,
  held: LandingChatPresentation | null,
  splitEnabled: boolean,
): ChatPresentation {
  if (held === 'terminal') return 'terminal'
  const wanted = held === 'chat' && chosen === 'terminal' ? 'chat' : chosen
  return wanted === 'split' && !splitEnabled ? 'chat' : wanted
}

export function useChatPresentation(
  shownChatId: string,
  splitContainerRef: {
    current: HTMLElement | null
  },
  held: LandingChatPresentation | null = null,
  recorded: LandingChatPresentation | null = null,
) {
  const seed = (chatId: string) =>
    lastChosen(chatId) ?? (IS_MOBILE ? 'chat' : (recorded ?? getDefaultChatPresentation()))
  const [chosen, setChosenRaw] = useState<ChatPresentation>(() => seed(shownChatId))
  const setChosen = useCallback(
    (next: ChatPresentation) => {
      rememberChosen(shownChatId, next)
      setChosenRaw(next)
    },
    [shownChatId],
  )
  const [heldBefore, setHeldBefore] = useState(held)
  if (heldBefore !== held) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: the same documented pattern as the re-seed below; an effect would paint the hidden pick for a frame first.
    setHeldBefore(held)
    if (heldBefore !== null && held === null) setChosenRaw(heldBefore)
  }
  const [recordedBefore, setRecordedBefore] = useState(recorded)
  if (recordedBefore !== recorded) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: the pattern above; an effect would paint the preference for a frame first.
    setRecordedBefore(recorded)
    if (recorded && !IS_MOBILE && !lastChosen(shownChatId)) setChosenRaw(recorded)
  }
  const splitEnabled = useSplitPresentationEnabled() && !IS_MOBILE
  const presentation = shownPresentation(chosen, held, splitEnabled)
  const splitting = presentation === 'split'

  const [returnOffered, setReturnOffered] = useState(false)
  const [splitFocus, setSplitFocus] = useState<'chat' | 'terminal'>('chat')

  const [seededFor, setSeededFor] = useState(shownChatId)
  if (seededFor !== shownChatId) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: React's documented "adjust state when a prop changes" pattern. An effect would paint the previous chat's surface for a frame first, which is the flicker this exists to avoid.
    setSeededFor(shownChatId)
    if (!splitting) setChosenRaw(seed(shownChatId))
    setReturnOffered(false)
    setSplitFocus('chat')
  }

  const [splitSizes, setSplitSizes] = useState<[number, number]>(SPLIT_DEFAULT_SIZES)
  const splitStacked = useSplitMeasure(splitting, splitContainerRef, deriveStacked, false)

  return {
    presentation,
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

export type PaneViewPresentation = 'side-by-side' | 'stacked' | 'tabs'

function derivePaneViewPresentation(width: number, height: number): PaneViewPresentation {
  if (width === 0 && height === 0) return 'side-by-side'
  if (Math.max(width, height) < SPLIT_SIDE_BY_SIDE_MIN_PX) return 'tabs'
  return width >= height ? 'side-by-side' : 'stacked'
}

export function usePaneViewPresentation(
  editorOpen: boolean,
  containerRef: RefObject<HTMLElement | null> | { current: HTMLElement | null },
): PaneViewPresentation {
  const measured = useSplitMeasure(
    editorOpen,
    containerRef,
    derivePaneViewPresentation,
    'side-by-side',
  )

  return editorOpen ? measured : 'tabs'
}

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
      if (!element || !canvas) return
      const slack = element.getBoundingClientRect().right - canvas.getBoundingClientRect().right
      setGridSlack(Math.max(0, Math.round(slack)))
    }

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

import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import {
  getDefaultChatPresentation,
  useSplitPresentationEnabled,
  type ChatPresentation,
  type LandingChatPresentation,
} from '@/features/settings/lib/chat-presentation'
import { IS_MOBILE } from '@/utils/platform'

export const SPLIT_MIN_HALF_PX = 340
export const SPLIT_SIDE_BY_SIDE_MIN_PX = 780
export const SPLIT_MIN_STACKED_PX = 160
export const SPLIT_DEFAULT_SIZES: [number, number] = [45, 55]

/** Measures the CONTAINER, never the window, so a narrow pane inside a wide
 *  window answers correctly. Reads `clientWidth`/`clientHeight` so a plain
 *  object double can drive it in tests. */
function useSplitMeasure<T>(
  active: boolean,
  containerRef: { current: HTMLElement | null },
  derive: (width: number, height: number) => T,
  initial: T,
): T {
  // Only the derived discrete answer is state, never raw pixels, so a sash drag
  // does not re-render the pane per pixel. `derive` is read through a ref so an
  // inline arrow cannot re-fire the effect.
  const deriveRef = useRef(derive)
  deriveRef.current = derive
  const [value, setValue] = useState<T>(initial)

  useEffect(() => {
    const container = containerRef.current
    if (!active || !container) return
    const measure = () => {
      const el = containerRef.current
      if (!el) return
      // A non-finite reading (a double supplying one axis) becomes 0 so NaN
      // never reaches the comparisons.
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

/** Each chat's last pick, at module scope because closing a pane deletes it and
 *  reopening mounts a fresh one; nothing renders off it, so it is not a store.
 *  Session-lifetime only. */
const lastChosenByChatId = new Map<string, ChatPresentation>()

/** Test fixtures reuse chat ids across `it` blocks; a real session never does. */
export function resetChatPresentationMemoryForTests(): void {
  lastChosenByChatId.clear()
}

/** Lands one just-minted chat on a surface before its pane mounts, without
 *  changing the global default for every other new chat. Landing-only: split
 *  is never where a chat opens by itself. */
export function presetChatLandingPresentation(
  chatId: string,
  presentation: LandingChatPresentation,
): void {
  lastChosenByChatId.set(chatId, presentation)
}

/** What shows: the daemon's view when it holds only one, else the user's pick. A split whose
 *  switch went away falls back to the chat. */
function shownPresentation(
  chosen: ChatPresentation,
  held: LandingChatPresentation | null,
  splitEnabled: boolean,
): ChatPresentation {
  if (held === 'terminal') return 'terminal'
  const wanted = held === 'chat' && chosen === 'terminal' ? 'chat' : chosen
  return wanted === 'split' && !splitEnabled ? 'chat' : wanted
}

/** Seeded from the preference, never subscribed, so an open chat keeps its surface when the
 *  setting changes. `held` is the daemon's view when it has only one (no hot-swap), else null;
 *  `recorded` is the surface the daemon remembers a hot-swap chat was last on, which a reload keeps. */
export function useChatPresentation(
  shownChatId: string,
  splitContainerRef: {
    current: HTMLElement | null
  },
  held: LandingChatPresentation | null = null,
  recorded: LandingChatPresentation | null = null,
) {
  // The phone opens on the chat: its TUI is a deliberate detour, never a recorded or default one.
  const seed = (chatId: string) =>
    lastChosenByChatId.get(chatId) ??
    (IS_MOBILE ? 'chat' : (recorded ?? getDefaultChatPresentation()))
  const [chosen, setChosenRaw] = useState<ChatPresentation>(() => seed(shownChatId))
  // Explicit picks are remembered per chat; the re-seed below restores through
  // `setChosenRaw` so a restore never rewrites the entry it reads.
  const setChosen = useCallback(
    (next: ChatPresentation) => {
      lastChosenByChatId.set(shownChatId, next)
      setChosenRaw(next)
    },
    [shownChatId],
  )
  // A provider that held one surface hands it over as the pick: the user was on it, and a
  // switch to a provider that holds none must not drop them on an unseen preference.
  const [heldBefore, setHeldBefore] = useState(held)
  if (heldBefore !== held) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: the same documented pattern as the re-seed below; an effect would paint the hidden pick for a frame first.
    setHeldBefore(held)
    if (heldBefore !== null && held === null) setChosenRaw(heldBefore)
  }
  // The daemon's record lands with the chat list, after a reload has mounted the pane.
  const [recordedBefore, setRecordedBefore] = useState(recorded)
  if (recordedBefore !== recorded) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: the pattern above; an effect would paint the preference for a frame first.
    setRecordedBefore(recorded)
    if (recorded && !IS_MOBILE && !lastChosenByChatId.has(shownChatId)) setChosenRaw(recorded)
  }
  // Two live halves do not fit a phone.
  const splitEnabled = useSplitPresentationEnabled() && !IS_MOBILE
  // Derived, not corrected: a reload, a disabled split or the daemon moving the chat can
  // never leave the pane on a surface that is not there.
  const presentation = shownPresentation(chosen, held, splitEnabled)
  const splitting = presentation === 'split'

  // Offered when Crowbar moved the user to the terminal and cannot move them
  // back without overriding their own choice.
  const [returnOffered, setReturnOffered] = useState(false)
  // xterm grabs focus whenever told it is active, so only the half holding the
  // caret is; the composer wins by default as the safer place for a stray keystroke.
  const [splitFocus, setSplitFocus] = useState<'chat' | 'terminal'>('chat')

  // Re-seed per chat, not per mount: the pane is retained across chat selection,
  // so the lazy initializer alone would ignore a changed preference.
  const [seededFor, setSeededFor] = useState(shownChatId)
  if (seededFor !== shownChatId) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: React's documented "adjust state when a prop changes" pattern. An effect would paint the previous chat's surface for a frame first, which is the flicker this exists to avoid.
    setSeededFor(shownChatId)
    // Split is a tool the user reached for, so it carries over to the next
    // chat; chat/terminal restore from that chat's own remembered pick.
    if (!splitting) setChosenRaw(seed(shownChatId))
    setReturnOffered(false)
    setSplitFocus('chat')
  }

  const [splitSizes, setSplitSizes] = useState<[number, number]>(SPLIT_DEFAULT_SIZES)
  // Zero width means "not measured yet", not narrow: answering stacked before
  // layout would flash the split vertical and then jump.
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

/** How a pane arranges its chat view against its editor view. */
export type PaneViewPresentation = 'side-by-side' | 'stacked' | 'tabs'

/** Too small on the longer axis downgrades to tabs while keeping the toggle, so
 *  growing the pane recovers the split; one floor serves both axes because a
 *  stacked layout needs height for the same reason side by side needs width. */
export function derivePaneViewPresentation(width: number, height: number): PaneViewPresentation {
  // Unmeasured: guess the common landscape case, so the pane does not pop
  // from one view to two when the real measurement lands.
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

/** How far xterm's grid stops short of its element (whole columns plus a
 *  scrollbar reservation), so overlays align with the text rather than the box.
 *  Measured because it varies with font metrics and pane width. */
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
      // No canvas yet: the MutationObserver calls back when it appears; a 0 here
      // would align to the element's edge instead of the grid's.
      if (!element || !canvas) return
      const slack = element.getBoundingClientRect().right - canvas.getBoundingClientRect().right
      setGridSlack(Math.max(0, Math.round(slack)))
    }

    // Mutations catch the canvas xterm builds asynchronously; resizes catch a
    // re-fit, measured a frame later because xterm re-fits after the container.
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

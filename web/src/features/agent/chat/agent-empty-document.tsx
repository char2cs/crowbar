import {
  useCallback,
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
} from 'react'
import type { ReactNode, Ref, RefObject } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { IS_MOBILE } from '@/utils/platform'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

/** The handle's own position on an empty document: the doc's top padding plus
 *  one line, matching `.doc`'s 48px / 14px × 1.7. `headerClearancePx` is
 *  `.doc`'s OWN extra top padding (composer.css's `--agent-header-clearance`,
 *  the same value agent-chat-pane.tsx computes for its overlay header) —
 *  when the document is genuinely empty this fallback has no rendered first
 *  line to measure, so it has to add that same clearance itself or it would
 *  place the handle above where the (padded-down) caret actually sits. */
function firstLineTop(headerClearancePx: number): number {
  return 48 + headerClearancePx + 23.8
}
/** The gap between the last line and the handle riding under it. */
const HANDLE_LEAD = 4

/**
 * Where the handle belongs: right under the LAST line of whatever is
 * written, never wherever the caret happens to be. Clicking back into the
 * middle of a paragraph to fix a word does not walk the controls up the page
 * with it — they stay put, because the box under them is still what sends.
 */
export function lastLineTop(doc: HTMLElement, headerClearancePx = 0): number {
  const editable = doc.querySelector<HTMLElement>('[data-slate-editor]')
  const last = editable?.lastElementChild
  if (!last || !editable?.textContent) return firstLineTop(headerClearancePx)
  return last.getBoundingClientRect().bottom - doc.getBoundingClientRect().top
}

export interface AgentEmptyDocumentHandle {
  /** The handle's own on-screen rect at this instant — read once, at the
   *  moment of the first send, to anchor the composer's arrival animation to
   *  wherever the eye already was. `null` before the first layout pass, or off
   *  a stale/unmounted node — a caller that gets it treats that as "nothing to
   *  arrive from" and skips the animation rather than guessing a position. */
  getHandleRect: () => DOMRect | null
}

export interface AgentEmptyDocumentProps {
  /** The document the composer's editor is written in, measured for its last line. */
  docRef: RefObject<HTMLDivElement | null>
  /** Whether the box holds text, from the editor's shape. */
  hasText: boolean
  onSubmit: () => void
  /** The selection chips and the surface switcher, left of the send button. */
  controls: ReactNode
  /** The composer's attach control, beside the send button. */
  attach: ReactNode
  working?: boolean
  canStop?: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onStop?: () => void
  /** `.doc`'s own extra top padding (`--agent-header-clearance`), which the
   *  empty fallback position must add itself. Defaults to 0. */
  headerClearancePx?: number
  /** Occupies the handle's own slot INSTEAD OF the model/effort/attach/send
   *  row, when this chat has a reason it cannot be typed into yet. There is only
   *  ever one thing in this slot. `undefined` renders the normal row. */
  banner?: ReactNode
  ref?: Ref<AgentEmptyDocumentHandle>
}

/**
 * A chat with nothing in it yet: a DOCUMENT, not an empty transcript with a box
 * under it. The composer's one editor is written in it; this is the handle that
 * rides right under the LAST line, never the caret, with the controls and send.
 */
export function AgentEmptyDocument({
  docRef,
  hasText,
  onSubmit,
  controls,
  attach,
  working = false,
  canStop = false,
  sending,
  onStop,
  headerClearancePx = 0,
  banner,
  ref,
}: AgentEmptyDocumentProps) {
  const handleRef = useRef<HTMLDivElement>(null)

  useImperativeHandle(
    ref,
    () => ({
      getHandleRect: () => handleRef.current?.getBoundingClientRect() ?? null,
    }),
    [],
  )

  const place = useCallback(() => {
    const doc = docRef.current
    const handle = handleRef.current
    if (!doc || !handle) return
    const top = lastLineTop(doc, headerClearancePx)
    handle.style.transform = `translateY(${Math.round(top + HANDLE_LEAD)}px)`
  }, [docRef, headerClearancePx])

  // Same frame as the line that moved it: the composer re-renders on the field's
  // height, observed before paint. An effect would lag the handle one frame.
  useLayoutEffect(place)

  // A caret that moves inside this document is the cheap signal for every edit
  // that does not change the height. Gated on the selection landing here:
  // `selectionchange` is document-wide, and Monaco fires it per pointer move.
  const onSelectionChange = useEffectEvent(() => {
    const anchor = document.getSelection()?.anchorNode ?? null
    if (!anchor || !docRef.current?.contains(anchor)) return
    place()
  })
  useEffect(() => {
    document.addEventListener('selectionchange', onSelectionChange)
    return () => document.removeEventListener('selectionchange', onSelectionChange)
  }, [])

  const empty = !hasText
  // Stopping wins even with text in the document: a turn already running must
  // keep its only way to be interrupted while the person writes.
  const stopping = working && canStop
  const sendingVisual = !stopping && empty && sending
  const idle = !stopping && !sendingVisual && empty

  return (
    <div ref={handleRef} className="dochandle" data-touch={IS_MOBILE || undefined}>
      <div className="inner">
        {banner ? (
          <div className="banner">{banner}</div>
        ) : (
          <div className="grp">
            <span className="side">{controls}</span>
            <span className="side">
              {attach}
              <button
                type="button"
                className={cn('send', stopping && 'halt', (idle || sendingVisual) && 'off')}
                disabled={idle || sendingVisual}
                aria-label={stopping ? 'Stop this turn' : sendingVisual ? 'Sending' : 'Send prompt'}
                title={
                  stopping ? 'Stop this turn — Esc' : sendingVisual ? 'Sending…' : 'Send — Enter'
                }
                onClick={stopping ? onStop : onSubmit}
              >
                {stopping ? (
                  <StopIcon size={16} />
                ) : sendingVisual ? (
                  <FlickerSpinner className="size-4" />
                ) : (
                  <UpIcon size={16} />
                )}
              </button>
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

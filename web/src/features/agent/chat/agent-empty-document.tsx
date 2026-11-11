import {
  useCallback,
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
} from 'react'
import type { ReactNode, Ref, RefObject } from 'react'
import { ComposerActionButton } from '@/features/agent/composer/composer-action-button'
import { IS_MOBILE } from '@/utils/platform'

// What a good first instruction covers, shown as ghost text until the first character is typed.
const GHOST_PROMPTS = [
  ['GOAL', 'What is different when this is finished.'],
  ['WHERE', 'Files, screens or branches involved. Type @ to point at one.'],
  ['KEEP', 'What must not change, and how you will check the result.'],
] as const

// An empty doc has no line to measure, so the handle sits at `.doc`'s top padding plus one line
// (the serif placeholder: 40px on desktop, 38px on the phone), plus `--agent-header-clearance`,
// which the caret is padded down by too.
function firstLineTop(headerClearancePx: number): number {
  return IS_MOBILE ? 30 + headerClearancePx + 38 : 48 + headerClearancePx + 40
}
/** The gap between the last line and the handle riding under it. */
const HANDLE_LEAD = 4

/** The handle rides under the LAST written line, not the caret: editing mid-paragraph must not
 *  walk the send controls up the page. */
export function lastLineTop(doc: HTMLElement, headerClearancePx = 0): number {
  const editable = doc.querySelector<HTMLElement>('[data-slate-editor]')
  const last = editable?.lastElementChild
  if (!last || !editable?.textContent) return firstLineTop(headerClearancePx)
  return last.getBoundingClientRect().bottom - doc.getBoundingClientRect().top
}

export interface AgentEmptyDocumentHandle {
  /** The handle's rect, read at the first send to anchor the composer's arrival animation.
   *  `null` before layout or once unmounted: the caller then skips the animation. */
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

/** A chat with nothing in it yet is a document the composer's editor writes in; this is the
 *  handle carrying the controls and send under its last line. */
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

  return (
    <div ref={handleRef} className="dochandle" data-touch={IS_MOBILE || undefined}>
      {!hasText && (
        <dl className="ghosts">
          {GHOST_PROMPTS.map(([label, hint]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{hint}</dd>
            </div>
          ))}
        </dl>
      )}
      <div className="inner">
        {banner ? (
          <div className="banner">{banner}</div>
        ) : (
          <div className="grp">
            <span className="side">{controls}</span>
            <span className="side">
              {attach}
              <ComposerActionButton
                hasText={hasText}
                working={working}
                canStop={canStop}
                sending={sending}
                onSend={onSubmit}
                onStop={onStop}
              />
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

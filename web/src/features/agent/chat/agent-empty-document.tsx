import { useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode, Ref } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  ChatMarkdownEditor,
  type CaretEdges,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

/** The handle's own position on an empty document: the doc's top padding plus
 *  one line, matching `.doc`'s 48px / 16px × 1.7. */
const FIRST_LINE_TOP = 48 + 27.2
/** The gap between the last line and the handle riding under it. */
const HANDLE_LEAD = 4

/**
 * Where the handle belongs: right under the LAST line of whatever is
 * written, never wherever the caret happens to be. Clicking back into the
 * middle of a paragraph to fix a word does not walk the controls up the page
 * with it — they stay put, because the box under them is still what sends.
 */
export function lastLineTop(doc: HTMLElement): number {
  const editable = doc.querySelector<HTMLElement>('[data-slate-editor]')
  const last = editable?.lastElementChild
  if (!last || !editable?.textContent) return FIRST_LINE_TOP
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
  /** The draft to OPEN with. The box owns its text after that. */
  draft: string
  /** Bumped when the draft is set from OUTSIDE the box, to remount it. */
  draftSeed: number
  /** Whether the box currently holds text — tracked live from the box's own
   *  onChange, unlike `draft`, which only carries what it was last OPENED
   *  with and goes stale the moment a person types for real. */
  hasText: boolean
  onDraftChange: (value: string) => void
  onSubmit: () => void
  /** The chat's own key handling — Enter, Tab, arrows, Escape. */
  onKeyDown: (
    event: ReactKeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => void
  /** The selection chips and the surface switcher, left of the send button. */
  controls: ReactNode
  working: boolean
  canStop: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onStop: () => void
  ref?: Ref<AgentEmptyDocumentHandle>
}

/**
 * A chat with nothing in it yet.
 *
 * It is a DOCUMENT, not an empty transcript with a message box under it. The
 * first thing a chat asks for is a description of the change you want, and that
 * is a piece of writing — so the blank chat gives it the whole pane at reading
 * measure and typographic size, and lets the controls come to the writing rather
 * than parking them in a bar at the bottom.
 *
 * The handle rides right under the LAST LINE, always — not the caret. Clicking
 * back into an earlier sentence to fix it does not drag the send button up
 * into the middle of the page with it; it stays where the document ends.
 *
 * Uncontrolled by design. React writes the text only when the incoming draft
 * genuinely differs from what the element holds — a controlled contenteditable
 * re-renders its own children out from under the selection and puts the caret
 * back at position zero on every keystroke.
 */
export function AgentEmptyDocument({
  draft,
  draftSeed,
  hasText,
  onDraftChange,
  onSubmit,
  onKeyDown,
  controls,
  working,
  canStop,
  sending,
  onStop,
  ref,
}: AgentEmptyDocumentProps) {
  const docRef = useRef<HTMLDivElement>(null)
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
    const top = lastLineTop(doc)
    handle.style.transform = `translateY(${Math.round(top + HANDLE_LEAD)}px)`
  }, [])

  // Same frame as the text that moved it. An effect would paint the handle at the
  // previous line for one frame, which reads as the bar lagging the content.
  useLayoutEffect(place)

  // Typing fires `selectionchange` as a side effect (the collapsed selection
  // moves with every keystroke) even though `place` itself no longer reads
  // it — cheaper than a MutationObserver, and it already covers every way
  // the last line can change: typing, deleting, pasting, undo.
  useEffect(() => {
    const onSelectionChange = () => place()
    document.addEventListener('selectionchange', onSelectionChange)
    return () => document.removeEventListener('selectionchange', onSelectionChange)
  }, [place])

  const empty = !hasText
  // STOPPING WINS EVEN WITH TEXT IN THE DOCUMENT — see composer-handle.tsx's
  // own note. A person typing while a turn is already running (a background
  // handoff, say — this surface can render before anything shows up in the
  // ledger) must not lose their only way to interrupt it just because they
  // started writing.
  const stopping = working && canStop
  // SENDING gets the same feedback composer-handle.tsx gives every later
  // message — this box owns only the FIRST one, but its own dispatch waits on
  // the identical round trip and used to show nothing at all for it.
  const sendingVisual = !stopping && empty && sending
  const idle = !stopping && !sendingVisual && empty

  return (
    <div className="docwrap" data-testid="agent-empty-document">
      <div ref={docRef} className="doc">
        <ChatMarkdownEditor
          key={draftSeed}
          initialValue={draft}
          placeholder="Describe the change…"
          ariaLabel="Describe the change"
          autoFocus
          onChange={onDraftChange}
          onKeyDown={onKeyDown}
          className="blk"
        />
      </div>
      <div ref={handleRef} className="dochandle">
        <div className="inner">
          <div className="grp">
            <span className="side">{controls}</span>
            <span className="side">
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
        </div>
      </div>
    </div>
  )
}

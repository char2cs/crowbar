import { useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode, Ref } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { ChatMarkdownEditor } from '@/features/agent/composer/plate/chat-markdown-editor'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

/** The lead the handle keeps below the caret when the document is empty: the
 *  doc's top padding plus one line, matching `.doc`'s 48px / 16px × 1.7. */
const FIRST_LINE_TOP = 48 + 27.2
/** The gap between the caret's line and the handle riding under it. */
const HANDLE_LEAD = 4

/**
 * Where the caret actually is.
 *
 * A COLLAPSED range usually reports no client rects at all — most often at the
 * end of a text node, which is where a caret spends nearly all its time. Without
 * the probe the handle silently falls back to the first line and parks itself on
 * top of everything written below it.
 */
function caretRect(range: Range): DOMRect | null {
  // Feature-detected, not assumed: jsdom implements neither Range geometry
  // method, so an unguarded call throws inside every test that types a prompt.
  const direct = typeof range.getClientRects === 'function' ? range.getClientRects()[0] : undefined
  if (direct?.height) return direct
  if (typeof range.insertNode !== 'function') return null
  const probe = document.createElement('span')
  // A zero-width space: it has a box to measure and no width to disturb the line.
  probe.textContent = '\u200b'
  range.insertNode(probe)
  const rect = probe.getBoundingClientRect()
  const parent = probe.parentNode
  probe.remove()
  // The insert split the text node in two; without this the block accumulates
  // fragments and every later range walks a different tree than it did before.
  parent?.normalize()
  return rect.height ? rect : null
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
  onKeyDown: (event: ReactKeyboardEvent<HTMLDivElement>, readMarkdown: () => string) => void
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
 * The handle rides ONE LINE BELOW THE CARET, which is the whole trick: it never
 * sits on the words being typed, and it is always where the hand already is.
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
    const selection = window.getSelection()
    let top = FIRST_LINE_TOP
    if (selection?.rangeCount && doc.contains(selection.anchorNode)) {
      const range = selection.getRangeAt(0).cloneRange()
      range.collapse(true)
      const rect = caretRect(range)
      if (rect?.height) top = rect.top - doc.getBoundingClientRect().top + rect.height
    } else {
      // No caret in the document — rest under the LAST line rather than the
      // first, so an unfocused draft does not hide the text it belongs to.
      const editable = doc.querySelector<HTMLElement>('[data-slate-editor]')
      const last = editable?.lastElementChild
      if (last && editable?.textContent) {
        top = last.getBoundingClientRect().bottom - doc.getBoundingClientRect().top
      }
    }
    handle.style.transform = `translateY(${Math.round(top + HANDLE_LEAD)}px)`
  }, [])

  // Same frame as the text that moved it. An effect would paint the handle at the
  // previous line for one frame, which reads as the bar lagging the caret.
  useLayoutEffect(place)

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
                title={stopping ? 'Stop this turn — Esc' : sendingVisual ? 'Sending…' : 'Send — Enter'}
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

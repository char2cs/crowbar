import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import type { DragEvent, KeyboardEvent as ReactKeyboardEvent, ReactNode, Ref } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'
import { ExcalidrawTakeover } from '@/features/agent/composer/excalidraw-takeover'
import { loadExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import {
  parseExcalidrawScene,
  type ParsedExcalidrawScene,
} from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import {
  ChatMarkdownEditor,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { cn } from '@/lib/utils'

/** The handle's own position on an empty document: the doc's top padding plus
 *  one line, matching `.doc`'s 48px / 14px × 1.7. */
const FIRST_LINE_TOP = 48 + 23.8
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
  /** Threaded straight through to `ChatMarkdownEditor`, which needs both to
   *  register paste interception (uploading a pasted image calls
   *  `uploadChatAttachment(wsId, chatId, ...)`). Optional to match
   *  `ChatMarkdownEditorProps` — see its own note. */
  wsId?: string
  chatId?: string
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
  wsId,
  chatId,
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
  const wrapRef = useRef<HTMLDivElement>(null)
  // Task 34's modals reach the box the same way the composer's own do
  // (`agent-composer.tsx`'s `editorRef`) — they sit as SIBLINGS of the
  // editor, outside `<Plate>`'s tree.
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const [modal, setModal] = useState<'excalidraw' | 'attach-file' | null>(null)
  const [excalidrawInitialScene, setExcalidrawInitialScene] = useState<
    ParsedExcalidrawScene | undefined
  >(undefined)
  const [dropTarget, setDropTarget] = useState(false)

  useImperativeHandle(
    ref,
    () => ({
      getHandleRect: () => handleRef.current?.getBoundingClientRect() ?? null,
    }),
    [],
  )

  const insertAttachmentMarkdown = useCallback((md: string) => {
    editorRef.current?.insertAttachmentMarkdown(md)
  }, [])
  const insertPendingImage = useCallback((objectUrl: string, alt: string) => {
    editorRef.current?.insertPendingImage(objectUrl, alt)
  }, [])
  const settlePendingImage = useCallback((objectUrl: string, finalMarkdown: string | null) => {
    editorRef.current?.settlePendingImage(objectUrl, finalMarkdown)
  }, [])

  // Attaching needs both ids — undefined here only for parity with
  // `ChatMarkdownEditorProps` (see its own note); the real call site
  // (`agent-chat-view.tsx`) always supplies both.
  const attachmentsReady = Boolean(wsId && chatId)
  const { uploadAndInsert } = useAttachmentUpload(
    wsId ?? '',
    chatId ?? '',
    insertAttachmentMarkdown,
    insertPendingImage,
    settlePendingImage,
  )

  // Same reasoning as agent-composer.tsx's own memoized drop handlers: an
  // inline arrow here would get a fresh identity on every render (this
  // component re-renders on every keystroke via `onDraftChange`), tearing
  // down and re-establishing Tauri's `onDragDropEvent` IPC subscription.
  const handleTauriDrop = useCallback(
    (paths: string[]) => {
      setDropTarget(false)
      if (!attachmentsReady) return
      for (const path of paths) void uploadAndInsert({ path })
    },
    [attachmentsReady, uploadAndInsert],
  )

  useTauriFileDrop(wrapRef, handleTauriDrop)

  const handleDragOver = useCallback((e: DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    setDropTarget(true)
  }, [])

  const handleDragLeave = useCallback((e: DragEvent) => {
    const related = e.relatedTarget as HTMLElement | null
    if (!related || !e.currentTarget.contains(related)) setDropTarget(false)
  }, [])

  // Plain-browser (non-Tauri dev) fallback — same as agent-composer.tsx's
  // own pill drop handler.
  const handleDrop = useCallback(
    (e: DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      if (!attachmentsReady) return
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [attachmentsReady, uploadAndInsert],
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
    <div
      ref={wrapRef}
      className={cn('docwrap', dropTarget && 'drop-target')}
      data-testid="agent-empty-document"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <div ref={docRef} className="doc">
        <ChatMarkdownEditor
          key={draftSeed}
          ref={editorRef}
          wsId={wsId}
          chatId={chatId}
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
              {attachmentsReady && (
                <ComposerPlusButton
                  onOpenExcalidraw={() => {
                    const saved = wsId && chatId ? loadExcalidrawDesign(wsId, chatId) : null
                    setExcalidrawInitialScene(
                      (saved ? parseExcalidrawScene(saved) : null) ?? undefined,
                    )
                    setModal('excalidraw')
                  }}
                  onOpenAttachFile={() => setModal('attach-file')}
                />
              )}
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
      {wsId && chatId && modal === 'attach-file' && (
        <AttachFileModal
          wsId={wsId}
          chatId={chatId}
          open
          onClose={() => setModal(null)}
          onInsertMarkdown={insertAttachmentMarkdown}
        />
      )}
      {wsId && chatId && modal === 'excalidraw' && (
        <ExcalidrawTakeover
          wsId={wsId}
          chatId={chatId}
          open
          onClose={() => setModal(null)}
          onInsertMarkdown={insertAttachmentMarkdown}
          initialScene={excalidrawInitialScene}
        />
      )}
    </div>
  )
}

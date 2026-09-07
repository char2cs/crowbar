import type { DragEvent, KeyboardEvent } from 'react'
import { useCallback, useContext, useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import type {
  AgentActivity,
  AgentTerminalWait,
  PermissionLevel,
} from '@/features/agent/api/agent-api'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { ComposerField } from '@/features/agent/composer/composer-field'
import { ComposerHalted } from '@/features/agent/composer/composer-halted'
import { ComposerHandle } from '@/features/agent/composer/composer-handle'
import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'
import { ExcalidrawTakeover } from '@/features/agent/composer/excalidraw-takeover'
import { loadExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import {
  parseExcalidrawScene,
  type ParsedExcalidrawScene,
} from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import type {
  CaretEdges,
  ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import {
  resolveComposerState,
  type ComposerRevival,
} from '@/features/agent/composer/lib/composer-state'
import { isMultiline } from '@/features/agent/composer/lib/handle-geometry'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { cn } from '@/lib/utils'

interface AgentComposerProps {
  wsId: string
  chatId: string
  activity: AgentActivity
  providerLabel: string
  /** This chat's current provider's own declared levels, for the permission
   *  switcher a permission choice offers. Undefined/empty hides the switcher. */
  permissionLevels?: PermissionLevel[]

  live: boolean
  working: boolean
  compacting: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  submitUnavailable: boolean
  terminalWait?: AgentTerminalWait
  /** The pane's own revive attempt, for a chat that is not live. */
  revival?: ComposerRevival
  haltedMessage?: string
  haltedResetsAt?: string
  canStop: boolean

  draft: string
  fieldHeight: number
  slashOpen: boolean
  onDraftChange: (value: string) => void
  onHeightChange: (height: number) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => void
  onSend: () => void
  onStop: () => void
  onOpenTerminal: () => void
  /** The manual retry for a revive that already gave up. */
  onRevive?: () => void
  /** Bumped when the draft is set from OUTSIDE the box, to remount it. */
  draftSeed: number
  /** The text that seed carries — see the note on `seed` in the view. */
  seedText: string
  /**
   * Where the excalidraw takeover portals to, instead of rendering inline.
   *
   * The composer lives inside `.dock`, a small bottom-pinned bar —
   * `position: absolute` itself, which makes it the CSS containing block for
   * any `position: absolute` descendant regardless of `.dock`'s own size.
   * Rendering the takeover inline there sized it to the dock, not the chat
   * pane (reported live: "not just where the input box is at"). The caller
   * (agent-chat-view.tsx) passes the `.agent-chat.chat` section itself, so
   * `inset-0` covers the whole pane. Undefined (tests that render this
   * component standalone) falls back to inline — harmless there since
   * nothing constrains its size in a bare test host.
   */
  takeoverContainer?: HTMLElement | null
}

/**
 * THE BAR.
 *
 * One 38px slot with exactly one occupant, resolved by `resolveComposerState`.
 * It is an input when you can talk, and it is the question, the permission, or
 * the reason you cannot, when you cannot — never an input rendered dead beneath
 * something else.
 */
export function AgentComposer(props: AgentComposerProps) {
  const [modal, setModal] = useState<'excalidraw' | 'attach-file' | null>(null)
  const [excalidrawInitialScene, setExcalidrawInitialScene] = useState<
    ParsedExcalidrawScene | undefined
  >(undefined)
  // Owned here, not by ComposerField — Tasks 29/34's modals sit as SIBLINGS
  // of the field below, outside `<Plate>`'s tree, and this is their only way
  // to reach the box's `insertAttachmentMarkdown`.
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const pillRef = useRef<HTMLDivElement>(null)
  const [dropTarget, setDropTarget] = useState(false)

  const insertAttachmentMarkdown = useCallback((md: string) => {
    editorRef.current?.insertAttachmentMarkdown(md)
  }, [])
  const insertPendingImage = useCallback((objectUrl: string, alt: string) => {
    editorRef.current?.insertPendingImage(objectUrl, alt)
  }, [])
  const settlePendingImage = useCallback((objectUrl: string, finalMarkdown: string | null) => {
    editorRef.current?.settlePendingImage(objectUrl, finalMarkdown)
  }, [])

  // Nullable, not the throwing `useWorkspaceStore()`: this component's own
  // tests render it bare (no provider) and must keep working — absent
  // context just means no cross-tab edit requests can reach it, same as
  // ExcalidrawPreview's own guard.
  const workspaceStore = useContext(WorkspaceStoreContext)
  const pendingExcalidrawEdit = useSyncExternalStore(
    useCallback(
      (onChange) => (workspaceStore ? workspaceStore.subscribe(onChange) : () => {}),
      [workspaceStore],
    ),
    () => workspaceStore?.getState().agentChats.excalidrawEditRequests[props.chatId],
  )
  useEffect(() => {
    if (!pendingExcalidrawEdit) return
    setExcalidrawInitialScene(pendingExcalidrawEdit)
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: not a derived copy of pendingExcalidrawEdit, an external-store event handler. Both setState calls run in the same batched effect invocation, so the modal never paints with a stale/absent scene; clearExcalidrawEditRequest is itself a side effect that can't happen during render.
    setModal('excalidraw')
    workspaceStore?.getState().clearExcalidrawEditRequest(props.chatId)
  }, [pendingExcalidrawEdit, props.chatId, workspaceStore])

  // Upload, CSV-inline resolution, and the failure toast all live in this one
  // shared hook now (also used by attach-file-modal.tsx and
  // agent-empty-document.tsx) — `void uploadAndInsert(...)` at every call
  // site below means nobody else is in a position to `.catch` this promise,
  // and the hook is what makes that safe.
  const { uploadAndInsert } = useAttachmentUpload(
    props.wsId,
    props.chatId,
    insertAttachmentMarkdown,
    insertPendingImage,
    settlePendingImage,
  )

  // Memoized: `useTauriFileDrop`'s own effect re-subscribes to Tauri's
  // `onDragDropEvent` (a dynamic import + async webview IPC registration)
  // whenever `onDrop`'s identity changes, and an inline arrow here would get
  // a fresh identity on every render — including every keystroke, since
  // `props.draft`/`onDraftChange` drive this component's re-renders.
  const handleTauriDrop = useCallback(
    (paths: string[]) => {
      setDropTarget(false)
      for (const path of paths) void uploadAndInsert({ path })
    },
    [uploadAndInsert],
  )

  useTauriFileDrop(pillRef, handleTauriDrop)

  const handlePillDragOver = useCallback((e: DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    setDropTarget(true)
  }, [])

  const handlePillDragLeave = useCallback((e: DragEvent) => {
    const related = e.relatedTarget as HTMLElement | null
    if (!related || !e.currentTarget.contains(related)) setDropTarget(false)
  }, [])

  // Plain-browser (non-Tauri dev) fallback: a real DataTransfer.files DOES
  // carry usable File bytes here — this is a completely different problem
  // from extractDroppedFilePaths's (which is about a host PATH, not bytes).
  const handlePillDrop = useCallback(
    (e: DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [uploadAndInsert],
  )

  const state = resolveComposerState({
    live: props.live,
    revival: props.revival,
    submitUnavailable: props.submitUnavailable,
    terminalWait: props.terminalWait,
    compacting: props.compacting,
    activity: props.activity,
    haltedMessage: props.haltedMessage,
    haltedResetsAt: props.haltedResetsAt,
  })

  switch (state.kind) {
    case 'signpost':
      return (
        <ComposerSignpost
          reason={state.reason}
          message={state.message}
          onOpenTerminal={props.onOpenTerminal}
          onRevive={props.onRevive}
        />
      )
    case 'choice':
      return (
        <ComposerChoice
          wsId={props.wsId}
          chatId={props.chatId}
          activity={props.activity}
          choice={state.choice}
          providerLabel={props.providerLabel}
          permissionLevels={props.permissionLevels}
          onOpenTerminal={props.onOpenTerminal}
        />
      )
    case 'halted':
      return <ComposerHalted message={state.message} resetsAt={state.resetsAt} />
    case 'compacting':
    case 'input': {
      // Compaction does not take the box away — it queues what is typed into it,
      // which is exactly what a busy turn does. The placeholder is the whole
      // difference, because a disabled field here would lose a thought somebody
      // is already halfway through writing.
      const placeholder =
        state.kind === 'compacting'
          ? 'Compacting… your message will be queued'
          : props.working
            ? 'Queue a message…'
            : 'Message the agent…'
      return (
        <>
          <div
            ref={pillRef}
            className={cn(
              'pill',
              isMultiline(props.fieldHeight) && 'multi',
              dropTarget && 'drop-target',
            )}
            onDragOver={handlePillDragOver}
            onDragLeave={handlePillDragLeave}
            onDrop={handlePillDrop}
          >
            <ComposerField
              key={props.draftSeed}
              ref={editorRef}
              wsId={props.wsId}
              chatId={props.chatId}
              initialValue={props.seedText}
              placeholder={placeholder}
              expanded={props.slashOpen}
              controls={props.slashOpen ? 'agent-skill-picker' : undefined}
              onChange={props.onDraftChange}
              onKeyDown={props.onKeyDown}
              onHeightChange={props.onHeightChange}
            />
            <ComposerHandle
              fieldHeight={props.fieldHeight}
              hasText={props.draft.trim().length > 0}
              working={props.working}
              canStop={props.canStop}
              sending={props.sending}
              onSend={props.onSend}
              onStop={props.onStop}
              onOpenExcalidraw={() => {
                const saved = loadExcalidrawDesign(props.wsId, props.chatId)
                setExcalidrawInitialScene((saved ? parseExcalidrawScene(saved) : null) ?? undefined)
                setModal('excalidraw')
              }}
              onOpenAttachFile={() => setModal('attach-file')}
            />
          </div>
          {modal === 'attach-file' && (
            <AttachFileModal
              wsId={props.wsId}
              chatId={props.chatId}
              open
              onClose={() => setModal(null)}
              onInsertMarkdown={(md) => editorRef.current?.insertAttachmentMarkdown(md)}
            />
          )}
          {modal === 'excalidraw' &&
            (() => {
              const takeover = (
                <ExcalidrawTakeover
                  wsId={props.wsId}
                  chatId={props.chatId}
                  open
                  onClose={() => setModal(null)}
                  onInsertMarkdown={(md) => editorRef.current?.insertAttachmentMarkdown(md)}
                  initialScene={excalidrawInitialScene}
                />
              )
              return props.takeoverContainer
                ? createPortal(takeover, props.takeoverContainer)
                : takeover
            })()}
        </>
      )
    }
  }
}

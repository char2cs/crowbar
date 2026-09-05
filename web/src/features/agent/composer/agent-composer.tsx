import type { DragEvent, KeyboardEvent } from 'react'
import { useCallback, useRef, useState } from 'react'
import type {
  AgentActivity,
  AgentTerminalWait,
  PermissionLevel,
} from '@/features/agent/api/agent-api'
import {
  uploadChatAttachment,
  type UploadChatAttachmentInput,
} from '@/features/agent/api/upload-chat-attachment'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { ComposerField } from '@/features/agent/composer/composer-field'
import { ComposerHalted } from '@/features/agent/composer/composer-halted'
import { ComposerHandle } from '@/features/agent/composer/composer-handle'
import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'
import { fileMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
import type {
  CaretEdges,
  ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import {
  resolveComposerState,
  type ComposerRevival,
} from '@/features/agent/composer/lib/composer-state'
import { isMultiline } from '@/features/agent/composer/lib/handle-geometry'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { toast } from '@/features/window/stores/toast-store'
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
  // Owned here, not by ComposerField — Tasks 29/34's modals sit as SIBLINGS
  // of the field below, outside `<Plate>`'s tree, and this is their only way
  // to reach the box's `insertAttachmentMarkdown`.
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const pillRef = useRef<HTMLDivElement>(null)
  const [dropTarget, setDropTarget] = useState(false)

  const insertUploaded = useCallback(
    (result: { ref: string; filename: string; contentType: string }) => {
      const md = result.contentType.startsWith('image/')
        ? imageMarkdown(result.filename, result.ref)
        : fileMarkdown(result.filename, result.ref)
      editorRef.current?.insertAttachmentMarkdown(md)
    },
    [],
  )

  // Caught here, not left to the caller: `void uploadAndInsert(...)` at every
  // call site means nobody is in a position to `.catch` this promise, and an
  // upload can fail for entirely ordinary reasons (offline, a daemon 413/500,
  // a revoked host-path read) — a dropped file that silently does nothing is
  // indistinguishable from a hang.
  const uploadAndInsert = useCallback(
    async (input: UploadChatAttachmentInput) => {
      try {
        const result = await uploadChatAttachment(props.wsId, props.chatId, input)
        insertUploaded(result)
      } catch (err) {
        toast.error(
          'Could not attach that file',
          err instanceof Error ? err.message : 'Crowbar could not reach the daemon — try again.',
        )
      }
    },
    [props.wsId, props.chatId, insertUploaded],
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
              onOpenExcalidraw={() => setModal('excalidraw')}
              onOpenAttachFile={() => setModal('attach-file')}
            />
          </div>
          {/* Task 29 (AttachFileModal) and Task 34 (ExcalidrawModal) each replace
              their `null` branch below with the real modal, wired to close via
              `setModal(null)` and insert via the imperative handle from Task 25. */}
          {modal === 'attach-file' && null}
          {modal === 'excalidraw' && null}
        </>
      )
    }
  }
}

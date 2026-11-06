import type { DragEvent, KeyboardEvent, ReactNode, Ref, RefObject } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { AgentTerminalWait } from '@/features/agent/api/agent-chat-reads'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { useAnswerTerminalWait } from '@/features/agent/hooks/use-answer-terminal-wait'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { ComposerField } from '@/features/agent/composer/composer-field'
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
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'
import {
  acceptsTyping,
  resolveComposerState,
  type ComposerState,
} from '@/features/agent/composer/lib/composer-state'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
} from '@/features/agent/chat/agent-empty-document'
import { isMultiline } from '@/features/agent/composer/lib/handle-geometry'
import { IS_MOBILE } from '@/utils/platform'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import type { AskItem } from '@/features/agent/stores/lib/transcript-items'
import {
  clearExcalidrawEditRequest,
  useExcalidrawEditRequests,
} from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { cn } from '@/lib/utils'

interface AgentComposerProps {
  wsId: string
  chatId: string
  /** The asks still pending, oldest first. */
  asks: AskItem[]
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
  /** Why the chat's feed failed, while it has. */
  feedError?: string
  canStop: boolean

  /** The blank chat's document, or the dock under a conversation: one editor either way. */
  placement: 'document' | 'dock'
  /** The box itself, for the caller to read on send and load text into. */
  editorRef: RefObject<ChatMarkdownEditorHandle | null>
  slashOpen: boolean
  onShapeChange: (shape: ComposerShape, edited: boolean) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
    shape: ComposerShape,
  ) => void
  onSend: () => void
  onStop: () => void
  onOpenTerminal: () => void
  /** The document's selection chips, left of its send button. */
  controls?: ReactNode
  /** The document's reason it cannot be typed into yet, in place of its controls. */
  banner?: ReactNode
  headerClearancePx?: number
  documentRef?: Ref<AgentEmptyDocumentHandle>
  /** Where the Excalidraw takeover portals to, so `inset-0` covers the chat pane: inline, the
   *  absolutely positioned `.dock` would be its containing block. Undefined renders inline. */
  takeoverContainer?: HTMLElement | null
}

// The bar: one 38px slot with one occupant from `resolveComposerState`. An input when you can
// talk; otherwise the question, the permission, or the reason you cannot, never a dead input.
export function AgentComposer(props: AgentComposerProps) {
  const [modal, setModal] = useState<'excalidraw' | 'attach-file' | null>(null)
  const [excalidrawInitialScene, setExcalidrawInitialScene] = useState<
    ParsedExcalidrawScene | undefined
  >(undefined)
  const { editorRef } = props
  const pillRef = useRef<HTMLDivElement>(null)
  const docRef = useRef<HTMLDivElement>(null)
  const [dropTarget, setDropTarget] = useState(false)
  const [fieldHeight, setFieldHeight] = useState(20)
  // Only the box's emptiness re-renders the bar; the text itself stays in the editor.
  const [empty, setEmpty] = useState(true)
  const { onShapeChange } = props
  const handleShape = useCallback(
    (shape: ComposerShape, edited: boolean) => {
      setEmpty(shape.empty)
      onShapeChange(shape, edited)
    },
    [onShapeChange],
  )

  const insertAttachmentMarkdown = useCallback(
    (md: string) => editorRef.current?.insertAttachmentMarkdown(md),
    [editorRef],
  )
  const insertPendingImage = useCallback(
    (objectUrl: string, alt: string) => editorRef.current?.insertPendingImage(objectUrl, alt),
    [editorRef],
  )
  const settlePendingImage = useCallback(
    (objectUrl: string, finalMarkdown: string | null) =>
      editorRef.current?.settlePendingImage(objectUrl, finalMarkdown),
    [editorRef],
  )

  const pendingExcalidrawEdit = useExcalidrawEditRequests((s) => s.requests[props.chatId])
  useEffect(() => {
    if (!pendingExcalidrawEdit) return
    setExcalidrawInitialScene(pendingExcalidrawEdit)
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: not a derived copy of pendingExcalidrawEdit, an external-store event handler. Both setState calls run in the same batched effect invocation, so the modal never paints with a stale/absent scene; clearExcalidrawEditRequest is itself a side effect that can't happen during render.
    setModal('excalidraw')
    clearExcalidrawEditRequest(props.chatId)
  }, [pendingExcalidrawEdit, props.chatId])

  // Upload, CSV inlining and the failure toast live in this one shared hook;
  // `void uploadAndInsert(...)` below is safe because the hook catches.
  const { uploadAndInsert } = useAttachmentUpload(
    props.wsId,
    props.chatId,
    insertAttachmentMarkdown,
    insertPendingImage,
    settlePendingImage,
  )

  // Memoized: the Tauri drop hook re-subscribes over IPC whenever its handler changes.
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
  // carry usable File bytes here, though never a host path.
  const handlePillDrop = useCallback(
    (e: DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [uploadAndInsert],
  )

  const openExcalidraw = () => {
    const saved = loadExcalidrawDesign(props.wsId, props.chatId)
    setExcalidrawInitialScene((saved ? parseExcalidrawScene(saved) : null) ?? undefined)
    setModal('excalidraw')
  }

  const state = resolveComposerState({
    live: props.live,
    submitUnavailable: props.submitUnavailable,
    terminalWait: props.terminalWait,
    compacting: props.compacting,
    asks: props.asks,
    feedError: props.feedError,
  })
  const document = props.placement === 'document'
  // The document always takes the first prompt; its banner says why it waits.
  const input = document || acceptsTyping(state)

  // A card that held the bar takes focus with it when it goes; hand it back to the
  // box, but never take it from wherever the person has since moved it.
  const wasInput = useRef(input)
  useEffect(() => {
    const returned = input && !wasInput.current
    wasInput.current = input
    const active = window.document.activeElement
    if (returned && (!active || active === window.document.body)) editorRef.current?.focus()
  }, [input, editorRef])

  // Compaction does not take the box away: it queues what is typed, as a busy
  // turn does, so only the placeholder differs.
  const placeholder = document
    ? 'Describe the change…'
    : state.kind === 'compacting'
      ? 'Compacting… your message will be queued'
      : props.working
        ? 'Queue a message…'
        : 'Message the agent…'

  const takeover = modal === 'excalidraw' && (
    <ExcalidrawTakeover
      wsId={props.wsId}
      chatId={props.chatId}
      open
      onClose={() => setModal(null)}
      onInsertMarkdown={insertAttachmentMarkdown}
      initialScene={excalidrawInitialScene}
    />
  )

  // One tree for every placement and state, so the editor never remounts: an
  // occupant hides the box instead of replacing it, and keeps what was typed.
  return (
    <>
      {!input && (
        <ComposerOccupant
          state={state}
          wsId={props.wsId}
          chatId={props.chatId}
          providerLabel={props.providerLabel}
          permissionLevels={props.permissionLevels}
          terminalWait={props.terminalWait}
          onOpenTerminal={props.onOpenTerminal}
        />
      )}
      <div
        ref={pillRef}
        hidden={!input}
        data-testid={document ? 'agent-empty-document' : undefined}
        className={cn(
          document ? 'docwrap' : 'pill',
          !document && isMultiline(fieldHeight) && 'multi',
          dropTarget && 'drop-target',
        )}
        data-touch={IS_MOBILE || undefined}
        onDragOver={handlePillDragOver}
        onDragLeave={handlePillDragLeave}
        onDrop={handlePillDrop}
      >
        <div ref={docRef} className={document ? 'doc' : undefined}>
          <ComposerField
            ref={editorRef}
            wsId={props.wsId}
            chatId={props.chatId}
            placement={props.placement}
            placeholder={placeholder}
            expanded={props.slashOpen}
            controls={props.slashOpen ? 'agent-skill-picker' : undefined}
            onShapeChange={handleShape}
            onKeyDown={props.onKeyDown}
            onHeightChange={setFieldHeight}
          />
        </div>
        {document ? (
          <AgentEmptyDocument
            ref={props.documentRef}
            docRef={docRef}
            hasText={!empty}
            controls={props.controls}
            attach={
              <ComposerPlusButton
                onOpenExcalidraw={openExcalidraw}
                onOpenAttachFile={() => setModal('attach-file')}
              />
            }
            working={props.working}
            canStop={props.canStop}
            sending={props.sending}
            onSubmit={props.onSend}
            onStop={props.onStop}
            headerClearancePx={props.headerClearancePx}
            banner={props.banner}
          />
        ) : (
          <ComposerHandle
            fieldHeight={fieldHeight}
            hasText={!empty}
            working={props.working}
            canStop={props.canStop}
            sending={props.sending}
            onSend={props.onSend}
            onStop={props.onStop}
            onOpenExcalidraw={openExcalidraw}
            onOpenAttachFile={() => setModal('attach-file')}
          />
        )}
      </div>
      {modal === 'attach-file' && (
        <AttachFileModal
          wsId={props.wsId}
          chatId={props.chatId}
          open
          onClose={() => setModal(null)}
          onInsertMarkdown={insertAttachmentMarkdown}
        />
      )}
      {takeover && props.takeoverContainer
        ? createPortal(takeover, props.takeoverContainer)
        : takeover}
    </>
  )
}

interface ComposerOccupantProps {
  state: ComposerState
  wsId: string
  chatId: string
  providerLabel: string
  permissionLevels?: PermissionLevel[]
  terminalWait?: AgentTerminalWait
  onOpenTerminal: () => void
}

/** What holds the bar when it is not an input: the question, or why not. */
function ComposerOccupant({ state, ...props }: ComposerOccupantProps) {
  const answerTerminalWait = useAnswerTerminalWait(props.wsId, props.chatId)
  switch (state.kind) {
    case 'signpost':
      return (
        <ComposerSignpost
          reason={state.reason}
          message={state.message}
          onOpenTerminal={props.onOpenTerminal}
          answers={props.terminalWait?.answers}
          onAnswer={answerTerminalWait}
        />
      )
    case 'choices':
      return (
        <div className="choice-stack" data-testid="agent-choice-stack">
          {state.asks.map((ask, i) => (
            <ComposerChoice
              key={ask.id}
              wsId={props.wsId}
              chatId={props.chatId}
              pending={state.asks.length}
              choice={ask.choice}
              toolTarget={ask.toolTarget}
              providerLabel={props.providerLabel}
              permissionLevels={props.permissionLevels}
              onOpenTerminal={props.onOpenTerminal}
              position={i + 1}
              request={{
                toolName: ask.choice.toolName,
                toolTarget: ask.toolTarget,
                toolInput: ask.toolInput,
                truncated: ask.toolInputTruncated,
                mcpServer: ask.mcpServer,
              }}
            />
          ))}
        </div>
      )
    default:
      return null
  }
}

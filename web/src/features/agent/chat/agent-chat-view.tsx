import { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, Ref } from 'react'
import {
  stopChat,
  type AgentChatMessage,
  type AgentInterruption,
  type AgentPromptResult,
  type AgentProvider,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-api'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import type { ComposerRevival } from '@/features/agent/composer/lib/composer-state'
import { ComposerSlashPicker } from '@/features/agent/composer/composer-slash-picker'
import type { CaretEdges } from '@/features/agent/composer/plate/chat-markdown-editor'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import type { DividerTag } from '@/features/agent/transcript/lib/flatten-transcript-rows'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
} from '@/features/agent/chat/agent-empty-document'
import { playArrival } from '@/features/agent/chat/lib/arrival-animation'
import { measureScrollbarWidth } from '@/features/agent/chat/lib/scrollbar-width'
import { useAgentActivity } from '@/features/agent/hooks/use-agent-activity'
import { useAgentTelemetry, limitResetsAt } from '@/features/agent/hooks/use-agent-telemetry'
import { useChatMessages } from '@/features/agent/hooks/use-chat-messages'
import { usePromptHistory } from '@/features/agent/hooks/use-prompt-history'
import { usePromptQueue } from '@/features/agent/hooks/use-prompt-queue'
import { useSlashCatalog } from '@/features/agent/hooks/use-slash-catalog'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

import '@/features/agent/styles/composer.css'
import '@/features/agent/styles/transcript.css'
import '@/features/agent/styles/activity.css'

/** One entry per progressive-blur layer in the dock's dissolve — see composer.css. */
const DISSOLVE_LAYERS = [0, 1, 2, 3, 4, 5, 6]

export interface AgentChatViewHandle {
  cancelUnsentPrompts: () => void
}

export interface AgentChatViewProps {
  wsId: string
  chatId: string
  providerId: string
  providers: AgentProvider[]
  /** Move this chat to another provider — the identity chip's other groups. */
  onSwitchProvider?: (providerId: string) => Promise<boolean>
  /** A switch is already running, or the pane is mid-delivery. */
  switchDisabled?: boolean
  working: boolean
  /** Is this chat LIVE mid-compaction right now — see WorkingLine's own prop
   *  doc for why this cannot be derived from `activity`. Feeds both the
   *  transcript's WorkingLine and the composer's own compacting state — one
   *  live source for both, where each used to read (or, for the composer,
   *  still needs) the ledger's permanently-dead-for-this interruption. */
  compacting?: boolean
  /** Increments for every lifecycle frame, including a batched fast turn. */
  turnRevision: number
  live: boolean
  /** The pane's own revive attempt, for a chat that is not live. */
  revival?: ComposerRevival
  /** The manual retry for a revive that already gave up. */
  onRevive?: () => void
  /** False while the native terminal presentation is selected. */
  active: boolean
  /** False for a retained, hidden tab. Network polling pauses in that state. */
  visible: boolean
  onOpenTerminal: () => void
  /** Whether the daemon has POSITIVELY established that this chat's CLI is
   *  blocked on a prompt Crowbar cannot answer. */
  terminalWaiting?: boolean
  terminalWaitKind?: string
  /** Client request ids the daemon has reported as delivered-and-over. */
  settledPrompts?: string[]
  /** The message(s) the agent is mid-way through saying — see useChatMessages. */
  streamingMessages?: { id: string; text: string }[]
  /** Prune confirmed ids out of the store's own streamingMessages[chatId] —
   *  see useChatMessages' onStreamingSettled for why this is safe where a
   *  turn-boundary clear was not. */
  onStreamingSettled?: (ids: string[]) => void
  onPromptSpawned: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  /** Re-read the server-folded busy value after a stale 409. */
  onRefreshChat: () => Promise<boolean>
  onQueueCountChange?: (count: number) => void
  onCancelableQueueCountChange?: (count: number) => void
  onDeliveryPendingChange?: (pending: boolean) => void
  /** Nothing has ever been said in this chat yet — `AgentEmptyDocument` is
   *  standing in for the dock, so the composer (and anything that lives only
   *  inside it, like a reviving/idle signpost) does not exist to be read. */
  onBlankChange?: (blank: boolean) => void
  /** The chat's sticky model / effort selection. '' means unset. */
  model: string
  effort: string
  onSelectionChange: (model: string, effort: string) => void
  /** Which surface the pane is showing, and how to change it. */
  presentation: ChatPresentation
  splitEnabled: boolean
  onSelectPresentation: (next: ChatPresentation) => void
  ref?: Ref<AgentChatViewHandle>
}

/** The provider's last word on why the chat stopped.
 *
 *  A `notice` is only the CURRENT state while nothing has happened after it — the
 *  next turn's own messages are the evidence that it no longer applies, so this
 *  reads the tail of the ledger rather than searching it. */
function haltedBy(messages: AgentChatMessage[]): AgentChatMessage | undefined {
  const last = messages.at(-1)
  return last?.role === 'notice' ? last : undefined
}

/** A message's or interruption's real display position — `displayOrder` when the
 *  backend reserved one, `sequence`/`seq` otherwise. Never compare `sequence`
 *  against `seq` (or either against the other's raw field) directly: an
 *  interruption recorded late (a switch's grace period, a gracefully-finishing
 *  stopped turn) can mint a `seq` that sorts it after activity it logically
 *  preceded — the exact reordering `displayOrder` exists to prevent. */
function displayOrderOf(item: { sequence: number; displayOrder?: number }): number
function displayOrderOf(item: { seq: number; displayOrder?: number }): number
function displayOrderOf(item: { sequence?: number; seq?: number; displayOrder?: number }): number {
  return item.displayOrder ?? item.sequence ?? item.seq ?? 0
}

/** The five interruption kinds the transcript draws a boundary pill for, mapped
 *  to that pill's own shape. `null` for everything else (permission,
 *  notification, elicitation) — those are answered inline, never a divider. */
function toDividerTag(interruption: AgentInterruption): DividerTag | null {
  switch (interruption.kind) {
    case 'compaction':
      return { kind: 'compaction', trigger: interruption.detail || 'auto' }
    case 'stopped':
      return { kind: 'interrupted' }
    case 'provider_switched':
      return { kind: 'provider', detail: interruption.detail ?? '' }
    case 'model_changed':
      return { kind: 'model', detail: interruption.detail ?? '' }
    case 'effort_changed':
      return { kind: 'effort', detail: interruption.detail ?? '' }
    default:
      return null
  }
}

/**
 * The chat surface: a transcript, and one bar under it.
 *
 * This file ASSEMBLES. Every piece of state it used to own lives in a hook, and
 * every piece of markup in a component — what is left is the wiring between
 * them, which is the only thing that genuinely belongs to "the chat view".
 */
export function AgentChatView({
  wsId,
  chatId,
  providerId,
  providers,
  onSwitchProvider,
  switchDisabled,
  working,
  compacting = false,
  turnRevision,
  live,
  revival,
  onRevive,
  active,
  visible,
  onOpenTerminal,
  terminalWaiting = false,
  terminalWaitKind,
  settledPrompts,
  streamingMessages,
  onStreamingSettled,
  onPromptSpawned,
  onPromptDispatchStart,
  onPromptDispatchSettled,
  onRefreshChat,
  onQueueCountChange,
  onCancelableQueueCountChange,
  onDeliveryPendingChange,
  onBlankChange,
  model,
  effort,
  onSelectionChange,
  presentation,
  splitEnabled,
  onSelectPresentation,
  ref,
}: AgentChatViewProps) {
  const activity = useAgentActivity(wsId, chatId, working, visible)
  const telemetry = useAgentTelemetry(wsId, chatId, visible)

  const [draft, setDraft] = useState('')
  // The box is UNCONTROLLED — a controlled contenteditable rebuilds itself under
  // the caret — so text pushed in from outside arrives by REMOUNT.
  //
  // THE SEED CARRIES ITS OWN TEXT rather than pointing at `draft`, because the
  // two are written by different things at nearly the same moment. The editor's
  // onChange lands asynchronously, so sending a prompt could bump the seed and
  // then have the outgoing text written back into `draft` before the new box
  // mounted — which mounted it holding the message that had just been sent.
  // Reading the seed's own text makes that ordering irrelevant.
  const [seed, setSeed] = useState({ text: '', n: 0 })
  // THE DOCK'S HEIGHT, published to CSS. The conversation runs UNDER the composer
  // and fades out behind it, so the scroller has to reserve exactly the room the
  // dock occupies — and the dock grows as the box does, so a constant would be
  // wrong the moment anybody typed a second line.
  const [dockHeight, setDockHeight] = useState(0)
  const dockObserver = useRef<ResizeObserver | null>(null)
  // THE SCROLLBAR'S OWN WIDTH, published to CSS the same way. The dissolve
  // spans the full pane (see composer.css) so its blur reaches exactly as far
  // as the transcript's real content does — never past it into the scrollbar
  // track, which is what made the glass read as smudging the thumb itself.
  const [scrollbarWidth, setScrollbarWidth] = useState(0)
  useEffect(() => setScrollbarWidth(measureScrollbarWidth()), [])
  // The empty document's own handle, read exactly once — at the instant of the
  // first send — so the arrival slide has something to arrive FROM. A ref, not
  // state: nothing ever renders off it, and it must survive the very unmount
  // (blank -> dock) that makes it useful.
  const emptyDocRef = useRef<AgentEmptyDocumentHandle>(null)
  const arrivalOriginRef = useRef<DOMRect | null>(null)
  // A ref CALLBACK, not a ref + effect: the dock is unmounted entirely on the
  // blank surface, and a callback is told about that directly instead of the
  // effect needing to depend on a branch decided further down the component.
  const dockRef = useCallback((node: HTMLDivElement | null) => {
    dockObserver.current?.disconnect()
    if (!node) {
      setDockHeight(0)
      return
    }
    // Consumed at most once per mount, and only when a first send just fired —
    // an already-populated chat that renders the dock straight away never sets
    // this, so `playArrival` no-ops for every ordinary open.
    playArrival(node, arrivalOriginRef.current)
    arrivalOriginRef.current = null
    const report = () => setDockHeight(node.getBoundingClientRect().height)
    report()
    const observer = new ResizeObserver(report)
    observer.observe(node)
    dockObserver.current = observer
  }, [])
  const seedDraft = useCallback((value: string) => {
    setDraft(value)
    setSeed((previous) => ({ text: value, n: previous.n + 1 }))
  }, [])
  const [fieldHeight, setFieldHeight] = useState(20)
  const [composerError, setComposerError] = useState('')
  const [submitUnavailable, setSubmitUnavailable] = useState(false)
  useEffect(() => setSubmitUnavailable(false), [chatId, providerId])

  // The queue baselines its evidence on the ledger's cursor and asks it to
  // re-read after every dispatch; the ledger's recovery walk asks the queue what
  // is still outstanding. Neither can be built before the other, so they meet
  // through this ref instead of through a hook order that cannot exist.
  const ledgerRef = useRef<{ cursor: number; refresh: () => void }>({
    cursor: 0,
    refresh: () => {},
  })
  const getBaseline = useCallback(() => ledgerRef.current.cursor, [])
  const refreshMessages = useCallback(() => ledgerRef.current.refresh(), [])
  const onSubmitUnavailable = useCallback(() => setSubmitUnavailable(true), [])

  const prompts = usePromptQueue({
    wsId,
    chatId,
    working,
    live,
    active,
    visible,
    turnRevision,
    terminalWaiting,
    settledPrompts,
    getBaseline,
    refreshMessages,
    onPromptSpawned,
    onPromptDispatchStart,
    onPromptDispatchSettled,
    onRefreshChat,
    onSubmitUnavailable,
  })

  const ledger = useChatMessages({
    wsId,
    chatId,
    providerId,
    visible,
    working,
    turnRevision,
    awaiting: prompts.deliveryPending,
    streamingMessages,
    onStreamingSettled,
    onApply: prompts.reconcile,
    pendingEvidence: prompts.pendingEvidence,
    pendingBaselines: prompts.pendingBaselines,
    onRecoveryExhausted: prompts.onRecoveryExhausted,
  })
  ledgerRef.current = { cursor: ledger.getCursor(), refresh: () => void ledger.refresh() }

  // Cold-open span: opened once per mount (this view remounts per chat via
  // `key={wsId:chatId}` in AgentChatPane), closed a frame after the ledger's
  // first page has painted. Mirrors workspace.switch's hydrate-to-pixels
  // pattern (workspace-view.tsx).
  useEffect(() => {
    markStart('chat.open')
  }, [])

  useEffect(() => {
    if (ledger.loading) return
    const raf = requestAnimationFrame(() => markEnd('chat.open'))
    return () => cancelAnimationFrame(raf)
  }, [ledger.loading])

  const slash = useSlashCatalog({ wsId, chatId, providerId, active, draft })

  // The currently-loaded window of the person's own words, oldest first — what
  // ArrowUp/ArrowDown actually walk. Never reaches past a page not yet loaded,
  // same as a terminal's history running out at the start of the session.
  const userTexts = useMemo(
    () => ledger.messages.filter((m) => m.role === 'user').map((m) => m.text),
    [ledger.messages],
  )
  const history = usePromptHistory(userTexts)

  useImperativeHandle(ref, () => ({ cancelUnsentPrompts: prompts.cancelUnsentPrompts }), [
    prompts.cancelUnsentPrompts,
  ])

  const { queue } = prompts
  useEffect(() => onQueueCountChange?.(queue.length), [queue.length, onQueueCountChange])
  useEffect(
    () => onCancelableQueueCountChange?.(prompts.cancelableCount),
    [prompts.cancelableCount, onCancelableQueueCountChange],
  )
  useEffect(
    () => onDeliveryPendingChange?.(prompts.deliveryPending),
    [prompts.deliveryPending, onDeliveryPendingChange],
  )

  const provider = providers.find((candidate) => candidate.id === providerId)
  const providerLabel = provider?.displayName ?? providerId
  // The provider's stop reason occupies the BAR, so the transcript must not also
  // render it as a row: it is one sentence, and saying it twice reads as the
  // provider having stopped twice.
  const halted = haltedBy(ledger.messages)
  // A stopped turn, positioned the exact same way a compaction is: a real,
  // sequence-anchored activity record (turn.RecordStop, backend-side), not
  // this session's own memory of having clicked Stop. That is what keeps it
  // from drifting — the old client-local version pinned itself to "the end of
  // the transcript" and every later message pushed it along.
  const stoppedInterruptions = useMemo(
    () => activity.interruptions.filter((interruption) => interruption.kind === 'stopped'),
    [activity.interruptions],
  )
  // Where the transcript draws its boundary pills — one merged wavy line per
  // anchor rather than one full-width divider per event (a stop, a switch and
  // a compaction landing on the same next message used to stack three
  // identical lines). Interruptions and messages share ONE sequence space, so
  // the anchor is simply the first message whose sequence is past the event's
  // — and several events can share that anchor, which is why this sorts them
  // ALL together first: a per-kind map (one for compaction, one for switches)
  // has no way to recover which of two DIFFERENT kinds actually happened
  // first, only this one, chronologically-sorted pass does.
  //
  // An event with nothing after it yet draws NOTHING, and for compaction that
  // is the point: the pill says "what is above me is gone from the model's
  // context", a claim about two sides. Drawing it under the newest message
  // would put a boundary below the whole conversation and read as if the
  // chat had ended.
  const eventsBefore = useMemo(() => {
    const marks: Record<number, DividerTag[]> = {}
    const relevant = activity.interruptions
      .map((interruption) => ({ interruption, tag: toDividerTag(interruption) }))
      .filter(
        (entry): entry is { interruption: AgentInterruption; tag: DividerTag } =>
          entry.tag !== null,
      )
      .sort((a, b) => displayOrderOf(a.interruption) - displayOrderOf(b.interruption))
    for (const { interruption, tag } of relevant) {
      const next = ledger.messages.find((m) => displayOrderOf(m) > displayOrderOf(interruption))
      if (!next) continue
      const list = marks[next.sequence] ?? []
      list.push(tag)
      marks[next.sequence] = list
    }
    return marks
  }, [activity.interruptions, ledger.messages])
  // The most recent stop with nothing after it yet: there is no next message to
  // anchor before, so this is the one case the divider still draws at the foot
  // of the transcript — exactly where the working line it replaced just was.
  const trailingInterruption = useMemo(() => {
    if (stoppedInterruptions.length === 0) return false
    const latest = stoppedInterruptions.reduce((a, b) =>
      displayOrderOf(b) > displayOrderOf(a) ? b : a,
    )
    return !ledger.messages.some((m) => displayOrderOf(m) > displayOrderOf(latest))
  }, [stoppedInterruptions, ledger.messages])

  const updateDraft = (value: string) => {
    setDraft(value)
    setComposerError('')
    slash.noteDraft(value)
    // A real edit abandons wherever history recall had gotten to — the next
    // ArrowUp starts a fresh walk from the newest turn, stashing THIS text.
    history.reset()
  }

  // `text` overrides the draft state for a surface that HOLDS its own text. The
  // document is a contenteditable, so the character that triggered the submit may
  // not have reached React yet — reading state there can enqueue the prompt one
  // keystroke short, or empty. The element is the authority; state is the mirror.
  const enqueueDraft = (text?: string) => {
    const result = prompts.enqueue(text ?? draft)
    if (!result.ok) {
      setComposerError(result.error)
      return
    }
    // Only ever meaningful for the chat's FIRST send: `blank` reads the render
    // this handler is still running in, before the queue push that is about to
    // flip it. A later send has no empty document mounted to read a handle off
    // — `emptyDocRef.current` is already null by then — so this is naturally a
    // no-op for every send after the first, with nothing here needing to know
    // which send it is.
    if (blank) arrivalOriginRef.current = emptyDocRef.current?.getHandleRect() ?? null
    seedDraft('')
    setComposerError('')
    slash.reset()
    // A recalled-but-unedited history entry can be sent as-is; without this a
    // later ArrowUp would resume from that stale index instead of the newest.
    history.reset()
  }

  const editPrompt = (item: PromptQueueItem) => {
    prompts.remove(item.clientRequestId)
    seedDraft(item.text)
    setComposerError('')
    history.reset()
  }

  // REGRESSION, live-verified: stopping a turn mid-GENERATION (as opposed to
  // mid-tool-call) can make the CLI exit outright rather than resume idle —
  // `live` drops, the composer swaps to its dormant signpost, and reattaching
  // remounts the field from `seed`, not from `draft`. Before Stop could fire
  // with text still in the box (see composer-handle.tsx), that remount always
  // found an EMPTY seed and nothing was lost. Now it can find a STALE one —
  // seeding wins, so whatever was mid-remount silently overwrote what the
  // person had just typed, and `draft` (never told about any of this) was left
  // pointing at text the box no longer held. Re-seeding with the box's own
  // current text here means a remount forced by any of this has the right
  // text to come back with, keeping `draft` and the box in agreement either way.
  const handleStop = () => {
    seedDraft(draft)
    void stopChat(wsId, chatId)
  }

  const selectSlashItem = (item: SlashCatalogItem) => {
    seedDraft(slash.accept(item))
  }

  const handleKeyDown = (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => {
    if (event.key === 'Escape' && slash.open) {
      event.preventDefault()
      slash.close()
      return
    }
    if (
      slash.open &&
      slash.items.length > 0 &&
      (event.key === 'ArrowDown' || event.key === 'ArrowUp')
    ) {
      event.preventDefault()
      slash.move(event.key === 'ArrowDown' ? 1 : -1)
      return
    }
    // Tab is the completion key, and it is unconditional while something is
    // highlighted: it never sends, so it is the one keystroke a user can press
    // without first working out what the picker currently thinks it has.
    if (event.key === 'Tab' && !event.shiftKey && slash.highlighted) {
      event.preventDefault()
      selectSlashItem(slash.highlighted)
      return
    }
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault()
      // Enter accepts a completion when there IS one, and otherwise sends. It
      // used to be swallowed for as long as the picker was open, which does not
      // survive the catalog being INCOMPLETE BY DECLARATION: no probe reports a
      // provider's own built-ins, so /compact, /clear and /context match nothing
      // and never will. Esc still closes the picker outright.
      if (slash.highlighted) {
        selectSlashItem(slash.highlighted)
        return
      }
      // The BOX's text, not the draft state. See ChatMarkdownEditor.
      enqueueDraft(readMarkdown())
      return
    }
    // History recall — a terminal's own ArrowUp/ArrowDown, only at an edge the
    // caret could not otherwise move past: intercepting anywhere else would
    // hijack ordinary vertical movement through a wrapped or multi-line draft.
    const plain = !event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
    if (!slash.open && plain && event.key === 'ArrowUp' && caret.atStart) {
      const recalled = history.recallOlder(readMarkdown())
      if (recalled !== undefined) {
        event.preventDefault()
        seedDraft(recalled)
      }
      return
    }
    if (!slash.open && plain && event.key === 'ArrowDown' && caret.atEnd) {
      const recalled = history.recallNewer()
      if (recalled !== undefined) {
        event.preventDefault()
        seedDraft(recalled)
      }
    }
  }

  // A chat with nothing in it is a DIFFERENT SURFACE, not an empty transcript
  // with a message box under it: the first thing it asks for is a description of
  // the change, which is writing, so it gets the pane at writing size. It stops
  // being one the instant anything is in flight — a queued prompt is already the
  // beginning of a conversation, and so is a failed load worth retrying.
  const nothingYet = ledger.messages.length === 0 && queue.length === 0
  const blank = !ledger.error && nothingYet
  useEffect(() => onBlankChange?.(blank), [blank, onBlankChange])
  // WHICH SURFACE IS NOT KNOWN UNTIL THE FIRST PAGE LANDS, and guessing shows the
  // wrong one: an empty ledger that has not answered yet is indistinguishable
  // from a chat with history, so picking either paints a composer the reader then
  // watches be replaced. The transcript's own loading state stands alone until
  // the answer arrives — one transition instead of two.
  const settling = ledger.loading && nothingYet

  const selectionCluster = (
    <SelectionCluster
      wsId={wsId}
      chatId={chatId}
      provider={provider}
      providers={providers}
      model={model}
      effort={effort}
      presentation={presentation}
      splitEnabled={splitEnabled && provider?.hotswap === true}
      showSwitcher={presentation !== 'terminal' && provider?.hasTerminal !== false}
      handoverBlocked={!provider?.hotswap && working}
      switchDisabled={switchDisabled}
      onSwitchProvider={onSwitchProvider}
      onSelectionChange={onSelectionChange}
      onSelectPresentation={onSelectPresentation}
    />
  )

  const transcript = (
    <AgentTranscript
      messages={ledger.messages}
      streamingBubbles={ledger.streamingBubbles}
      queue={queue}
      providers={providers}
      activity={activity}
      // WorkingLine itself now carves compaction out of its own "blocked on a
      // person" check — excluding it here too used to be the only thing
      // silencing it, and would make that carve-out unreachable.
      working={working}
      compacting={compacting}
      loading={ledger.loading}
      error={ledger.error}
      hasOlder={ledger.hasOlder}
      onLoadOlder={() => void ledger.loadOlder()}
      onRetryLoad={() => void ledger.loadInitial()}
      onOpenTerminal={onOpenTerminal}
      onEditPrompt={editPrompt}
      onCancelPrompt={prompts.remove}
      onRetryPrompt={prompts.retry}
      showTerminalHintFor={
        prompts.showAwaitingTerminalHint ? prompts.awaitingHead?.clientRequestId : undefined
      }
      eventsBefore={eventsBefore}
      suppressSequence={halted?.sequence}
      trailingInterruption={trailingInterruption}
      dockHeight={dockHeight}
    />
  )

  if (settling) {
    return (
      <section className="agent-chat chat" aria-label="Agent chat">
        {transcript}
      </section>
    )
  }

  if (blank) {
    return (
      <section className="agent-chat chat" aria-label="Agent chat">
        <AgentEmptyDocument
          ref={emptyDocRef}
          draft={seed.text}
          draftSeed={seed.n}
          hasText={draft.trim().length > 0}
          onDraftChange={updateDraft}
          onSubmit={() => enqueueDraft()}
          onKeyDown={handleKeyDown}
          controls={selectionCluster}
          working={working}
          canStop={live}
          sending={prompts.deliveryPending}
          onStop={handleStop}
        />
        {composerError && (
          <p className="meta" role="alert">
            {composerError}
          </p>
        )}
      </section>
    )
  }

  return (
    <section
      className="agent-chat chat"
      aria-label="Agent chat"
      style={
        {
          '--agent-dock-h': `${Math.round(dockHeight)}px`,
          '--agent-scrollbar-w': `${scrollbarWidth}px`,
        } as React.CSSProperties
      }
    >
      {transcript}

      <div className="dissolve" aria-hidden="true">
        {DISSOLVE_LAYERS.map((_, i) => (
          <div key={i} className="dissolve-layer" />
        ))}
      </div>

      <div ref={dockRef} className="dock">
        <SubagentShelf activity={activity} />
        {slash.open && (
          <ComposerSlashPicker
            state={slash.state}
            items={slash.items}
            selected={slash.selected}
            onSelect={selectSlashItem}
          />
        )}
        <AgentComposer
          wsId={wsId}
          chatId={chatId}
          activity={activity}
          providerLabel={providerLabel}
          permissionLevels={provider?.permissionLevels}
          live={live}
          revival={revival}
          working={working}
          compacting={compacting}
          sending={prompts.deliveryPending}
          submitUnavailable={submitUnavailable}
          terminalWait={terminalWaiting ? { kind: terminalWaitKind ?? '' } : undefined}
          haltedMessage={halted?.text}
          haltedResetsAt={limitResetsAt(telemetry)}
          canStop={live}
          draft={draft}
          fieldHeight={fieldHeight}
          slashOpen={slash.open}
          onDraftChange={updateDraft}
          onHeightChange={setFieldHeight}
          onKeyDown={handleKeyDown}
          onSend={() => enqueueDraft()}
          onStop={handleStop}
          onOpenTerminal={onOpenTerminal}
          onRevive={onRevive}
          draftSeed={seed.n}
          seedText={seed.text}
        />
        <ProviderBar
          wsId={wsId}
          chatId={chatId}
          provider={provider}
          providers={providers}
          onSwitchProvider={onSwitchProvider}
          switchDisabled={switchDisabled}
          model={model}
          effort={effort}
          telemetry={telemetry}
          presentation={presentation}
          splitEnabled={splitEnabled && provider?.hotswap === true}
          queued={queue.length}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
          showSwitcher={presentation !== 'terminal' && provider?.hasTerminal !== false}
          handoverBlocked={!provider?.hotswap && working}
        />
        {(composerError || prompts.persistenceLost) && (
          <p className="meta" role="alert">
            {composerError ||
              'Pending prompts cannot be saved on this device. Keep Crowbar open until they finish.'}
          </p>
        )}
      </div>
    </section>
  )
}

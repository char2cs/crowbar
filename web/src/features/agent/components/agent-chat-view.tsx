import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type Ref,
} from 'react'
import { ArrowUpIcon, PencilIcon, RotateCcwIcon, TerminalIcon, XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { Textarea } from '@/components/ui/textarea'
import {
  getSlashCatalog,
  listChatMessages,
  submitAgentPrompt,
  type AgentChatMessage,
  type AgentPromptResult,
  type AgentProvider,
  type SlashCatalog,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-api'
import {
  canPersistPromptQueue,
  isPromptTextWithinLimit,
  loadPromptQueue,
  MAX_PROMPT_QUEUE_ITEMS,
  savePromptQueue,
  type PromptQueueItem,
} from '@/features/agent/lib/prompt-queue-persistence'
import { MarkdownPreview } from '@/features/panes/lib/markdown'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'
import {
  AgentActivityStrip,
  AgentTurnTools,
} from '@/features/agent/components/agent-activity-strip'
import { AgentChoicePrompts } from '@/features/agent/components/agent-choice-prompt'
import { AgentContextGauge } from '@/features/agent/components/agent-context-gauge'
import { AgentModelPicker } from '@/features/agent/components/agent-model-picker'
import { useAgentActivity } from '@/features/agent/components/use-agent-activity'

const MESSAGE_PAGE_SIZE = 100
const MESSAGE_POLL_MS = 1_000
const BUSY_RECHECK_MS = 1_000
const EVIDENCE_RECOVERY_MAX_PAGES = 100
const SLASH_DEBOUNCE_MS = 150
const AWAITING_TERMINAL_HINT_MS = 6_000

export interface AgentChatViewHandle {
  cancelUnsentPrompts: () => void
}

interface AgentChatViewProps {
  wsId: string
  chatId: string
  providerId: string
  providers: AgentProvider[]
  working: boolean
  /** Increments for every lifecycle frame, including a batched fast turn. */
  turnRevision: number
  live: boolean
  /** False while the native terminal presentation is selected. */
  active: boolean
  /** False for a retained, hidden tab. Network polling pauses in that state. */
  visible: boolean
  onOpenTerminal: () => void
  /**
   * Whether the daemon has POSITIVELY established that this chat's CLI is blocked
   * on a prompt Crowbar cannot answer.
   *
   * It is here to switch OFF the timed guess below (see showAwaitingTerminalHint):
   * that hint exists only because there used to be no way to know, and saying
   * "the provider MAY be waiting" beside a banner that says it definitely is
   * would be two voices on one fact, one of them hedging.
   */
  terminalWaiting?: boolean
  /**
   * Client request ids the daemon has reported as delivered-and-over.
   *
   * A pending item is normally resolved by its own text arriving in the ledger as
   * a user message. A provider's own built-in command never produces one — the CLI
   * handles it and announces nothing — so this is the only evidence that will ever
   * come for those, and without it the FIFO head blocks the composer forever.
   */
  settledPrompts?: string[]
  /**
   * The assistant message being produced RIGHT NOW, if any.
   *
   * It is not in `messages` and never will be under this identity: the ledger
   * receives the message once, complete, and this is the view of it growing in
   * the meantime. Rendered below the recorded messages and dropped the moment the
   * finished one arrives.
   */
  streamingMessageId?: string
  streamingMessageText?: string
  onPromptSpawned: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  /** Re-read the server-folded busy value after a stale 409. */
  onRefreshChat: () => Promise<boolean>
  onQueueCountChange?: (count: number) => void
  onCancelableQueueCountChange?: (count: number) => void
  /** True while a submitted prompt still needs authoritative hook confirmation. */
  onDeliveryPendingChange?: (pending: boolean) => void
  /** The chat's sticky model / effort selection. '' means unset — the provider's
   *  own default, which is a real answer and not a value to render as chosen. */
  model: string
  effort: string
  /** An accepted selection, for the owner of the chat to write back. */
  onSelectionChange: (model: string, effort: string) => void
  ref?: Ref<AgentChatViewHandle>
}

type CatalogState =
  | { state: 'closed' }
  | { state: 'loading' }
  | { state: 'ready'; catalog: SlashCatalog }
  | { state: 'error'; error: Error; unavailable: boolean }

function requestId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') return globalThis.crypto.randomUUID()
  if (typeof globalThis.crypto?.getRandomValues !== 'function') {
    throw new Error('Secure request identity is unavailable in this webview.')
  }
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0'))
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex
    .slice(6, 8)
    .join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

function mergeMessages(current: AgentChatMessage[], incoming: AgentChatMessage[]) {
  const bySequence = new Map(current.map((item) => [item.sequence, item]))
  for (const item of incoming) bySequence.set(item.sequence, item)
  return [...bySequence.values()].sort((a, b) => a.sequence - b.sequence)
}

/** A queue item that has been handed to the server and is waiting to be proven
 *  delivered. Only these are resolvable by evidence; the rest are the user's. */
function awaitingEvidence(item: PromptQueueItem): boolean {
  return (
    item.state === 'submitting' ||
    item.state === 'awaiting_turn' ||
    item.state === 'outcome_uncertain'
  )
}

function samePrompt(message: AgentChatMessage, prompt: PromptQueueItem): boolean {
  return (
    message.role === 'user' &&
    message.sequence > prompt.baselineSequence &&
    message.text.trim() === prompt.text.trim()
  )
}

function completenessCopy(completeness: SlashCatalog['completeness']): string {
  switch (completeness) {
    case 'complete':
      return 'Complete skill catalog'
    case 'model_visible':
      return 'Model-visible skills; the native menu may include more'
    case 'plugin_only':
      return 'Plugin skills; the native menu may include more'
  }
}

function providerName(providers: AgentProvider[], id: string): string {
  return providers.find((provider) => provider.id === id)?.displayName ?? id
}

/** One recorded turn.
 *
 *  Four roles reach here, and the discriminator used to be a single `user`
 *  boolean because there were only two. The other two are both "text in this
 *  conversation that NOBODY in it typed", and they are separate roles because
 *  they answer to different people:
 *
 *    - `harness` is the provider's own machinery talking to its own model — a
 *      background-subagent completion report is the measured case. The agent
 *      genuinely received it, so it is shown; the human did not write it, so it
 *      must never wear the user's bubble. Anything else quotes a person saying
 *      something they never said.
 *    - `notice` is Crowbar relaying a provider's OWN words about why a chat
 *      stopped ("You've hit your usage limit…"). It is the same class of thing
 *      to the reader as the terminal-wait banner and the interruption strip —
 *      the agent has stopped and something is wanted — so it wears that family's
 *      card and announces itself.
 *
 *  A role this build has never heard of still renders its text, unstyled and
 *  verbatim. A message that vanishes is strictly worse than one that looks
 *  plain: the ledger recorded something the agent acted on, and a client too old
 *  to name it is not a reason to hide it.
 *
 *  Everything here is derived from props. The pane this list lives in is RETAINED
 *  across chat selection, so a row holding state of its own would carry one
 *  chat's UI onto another chat's transcript. */
function MessageRow({
  message,
  showProvider,
  providers,
}: {
  message: AgentChatMessage
  showProvider: boolean
  providers: AgentProvider[]
}) {
  const user = message.role === 'user'
  const assistant = message.role === 'assistant'
  const harness = message.role === 'harness'
  const notice = message.role === 'notice'
  return (
    <article
      className={cn('flex w-full', user ? 'justify-end' : 'justify-start')}
      data-sequence={message.sequence}
      data-testid={`agent-message-${message.sequence}`}
      data-role={message.role}
    >
      <div
        // A notice is a live announcement, not prose — the reader may already be
        // scrolled away when the provider stops the chat, and it is the only row
        // here that anybody needs told about.
        role={notice ? 'alert' : undefined}
        className={cn(
          'min-w-0 max-w-[88%] text-sm',
          user && 'rounded-2xl rounded-br-md bg-muted px-4 py-2.5 text-foreground',
          harness && 'w-full rounded-lg border border-border bg-muted/40 px-3 py-2',
          notice && 'w-full rounded-lg border border-warning/40 bg-warning/10 px-3 py-2',
          !user && !harness && !notice && 'w-full py-1 text-foreground',
        )}
      >
        {showProvider && (
          <p className="mb-1.5 text-muted-foreground text-xs">
            {providerName(providers, message.providerId)}
          </p>
        )}
        {/* Said in words, not only in styling. The body of one of these is raw
            provider markup a reader has every reason to mistake for their own
            last message, and a muted box alone does not say whose words they
            are. */}
        {harness && (
          <p className="mb-1.5 text-muted-foreground text-xs" data-testid="message-harness-label">
            Sent to the agent by {providerName(providers, message.providerId)} — not by you
          </p>
        )}
        {assistant ? (
          <MarkdownPreview className="break-words text-sm">{message.text}</MarkdownPreview>
        ) : (
          // Verbatim for everything the model did not write. A harness payload is
          // markup — `<task-notification>` is an HTML tag to a markdown renderer,
          // which would swallow the row whole — and a notice is a provider's exact
          // sentence, which is the one thing about it worth showing.
          <p
            className={cn(
              'whitespace-pre-wrap break-words',
              harness && 'text-muted-foreground',
              notice && 'text-warning-foreground',
            )}
          >
            {message.text}
          </p>
        )}
        {/* Provenance, not a headline: what the CLI ITSELF said it ran this turn at.
            It is a different fact from the chat's requested selection — the two can
            legitimately disagree — so it is only ever shown for a turn the provider
            actually reported one on. */}
        {assistant && message.effort && (
          <p
            className="mt-1 text-muted-foreground text-xs"
            title="Reasoning effort the provider reported for this turn"
            data-testid="message-effort"
          >
            {message.effort} effort
          </p>
        )}
      </div>
    </article>
  )
}

function QueueRow({
  item,
  showTerminalHint,
  onEdit,
  onCancel,
  onRetry,
  onOpenTerminal,
}: {
  item: PromptQueueItem
  showTerminalHint: boolean
  onEdit: () => void
  onCancel: () => void
  onRetry: () => void
  onOpenTerminal: () => void
}) {
  const uncertain = item.state === 'outcome_uncertain'
  const failed = item.state === 'failed' || uncertain
  const status =
    item.state === 'queued'
      ? 'Queued'
      : item.state === 'submitting'
        ? 'Submitting…'
        : item.state === 'awaiting_turn'
          ? 'Waiting for provider confirmation…'
          : uncertain
            ? 'Delivery outcome unknown'
            : 'Failed'

  return (
    <article
      className={cn(
        'ml-auto w-[88%] rounded-2xl rounded-br-md border border-dashed px-4 py-2.5 text-sm',
        failed ? 'border-destructive/50 bg-destructive/5' : 'border-border bg-muted/30',
      )}
      data-client-request-id={item.clientRequestId}
      data-state={item.state}
      data-testid="queued-prompt"
    >
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <span className={cn('text-xs', failed ? 'text-destructive' : 'text-muted-foreground')}>
          {status}
        </span>
        {(item.state === 'queued' || failed) && (
          <div className="flex items-center gap-0.5">
            {failed && (
              <Button
                size="icon-xs"
                variant="ghost"
                tooltip={uncertain ? 'Retry with the same request ID' : 'Retry'}
                aria-label={uncertain ? 'Retry same request' : 'Retry prompt'}
                onClick={onRetry}
              >
                <RotateCcwIcon />
              </Button>
            )}
            {!uncertain && (
              <Button
                size="icon-xs"
                variant="ghost"
                tooltip="Edit"
                aria-label="Edit queued prompt"
                onClick={onEdit}
              >
                <PencilIcon />
              </Button>
            )}
            <Button
              size="icon-xs"
              variant="ghost"
              tooltip="Cancel"
              aria-label="Cancel queued prompt"
              onClick={onCancel}
            >
              <XIcon />
            </Button>
          </div>
        )}
      </div>
      <p className="whitespace-pre-wrap break-words">{item.text}</p>
      {item.error && <p className="mt-2 text-destructive text-xs">{item.error}</p>}
      {uncertain && (
        <p className="mt-2 text-muted-foreground text-xs">
          Check the hook-confirmed messages or native terminal before retrying. Retrying preserves
          this request’s identity.
        </p>
      )}
      {showTerminalHint && (
        <div className="mt-2 flex items-center justify-between gap-3 rounded-lg border bg-background/60 px-2.5 py-2 text-xs">
          <p className="text-muted-foreground">
            The provider may be waiting for a trust or permission answer in its native terminal.
          </p>
          <Button className="shrink-0" size="xs" variant="secondary" onClick={onOpenTerminal}>
            <TerminalIcon /> Open Terminal
          </Button>
        </div>
      )}
    </article>
  )
}

function SlashPicker({
  state,
  items,
  selected,
  onSelect,
  onOpenTerminal,
}: {
  state: CatalogState
  items: SlashCatalogItem[]
  selected: number
  onSelect: (item: SlashCatalogItem) => void
  onOpenTerminal: () => void
}) {
  return (
    <div
      id="agent-skill-picker"
      className="absolute inset-x-0 bottom-[calc(100%+0.5rem)] z-20 max-h-72 overflow-y-auto rounded-xl border bg-popover p-1.5 text-popover-foreground shadow-lg"
      data-testid="slash-picker"
      role="listbox"
      aria-label="Available skills"
    >
      {state.state === 'loading' && (
        <div className="flex items-center gap-2 px-3 py-5 text-muted-foreground text-sm">
          <FlickerSpinner className="size-4" /> Discovering skills… Enter still sends.
        </div>
      )}
      {state.state === 'error' && (
        <div className="space-y-3 px-3 py-3 text-sm">
          <p>
            {state.unavailable ? 'This provider has no React skill catalog.' : state.error.message}
          </p>
          <Button size="sm" variant="secondary" onClick={onOpenTerminal}>
            <TerminalIcon /> Open Terminal
          </Button>
        </div>
      )}
      {state.state === 'ready' && (
        <>
          <div className="flex items-start justify-between gap-2 px-2 py-1.5">
            <p className="text-muted-foreground text-xs">
              {completenessCopy(state.catalog.completeness)}
            </p>
            <Badge variant="outline" size="sm">
              {state.catalog.items.length}
            </Badge>
          </div>
          {state.catalog.warnings.length > 0 && (
            <p className="px-2 pb-1.5 text-warning-foreground text-xs">
              {state.catalog.warnings[0]}
              {state.catalog.warnings.length > 1
                ? ` (+${state.catalog.warnings.length - 1} more)`
                : ''}
            </p>
          )}
          {items.length === 0 ? (
            <p className="px-3 py-5 text-center text-muted-foreground text-sm">
              No matching skill. Press Enter to send it to the provider anyway — its own commands
              are not in this catalog.
            </p>
          ) : (
            items.map((item, index) => (
              <button
                key={item.id}
                type="button"
                role="option"
                aria-selected={index === selected}
                className={cn(
                  'block w-full rounded-lg px-3 py-2 text-left outline-none',
                  index === selected ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/60',
                )}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => onSelect(item)}
              >
                <span className="block font-medium text-sm">{item.label}</span>
                {(item.description || item.source) && (
                  <span className="mt-0.5 block truncate text-muted-foreground text-xs">
                    {item.description || item.source}
                  </span>
                )}
              </button>
            ))
          )}
          {state.catalog.completeness !== 'complete' && (
            <button
              type="button"
              className="mt-1 flex w-full items-center gap-2 rounded-lg border-t px-3 py-2 text-left text-muted-foreground text-xs hover:bg-accent hover:text-foreground"
              onClick={onOpenTerminal}
            >
              <TerminalIcon className="size-3.5" /> Open the native menu for the complete provider
              surface
            </button>
          )}
        </>
      )}
    </div>
  )
}

// react-doctor-disable-next-line no-giant-component -- cohesive delivery state machine: ledger reconciliation, durable FIFO, and slash request cancellation share ordering authority.
export function AgentChatView({
  wsId,
  chatId,
  providerId,
  providers,
  working,
  turnRevision,
  live,
  active,
  visible,
  onOpenTerminal,
  terminalWaiting = false,
  settledPrompts,
  streamingMessageId,
  streamingMessageText,
  onPromptSpawned,
  onPromptDispatchStart,
  onPromptDispatchSettled,
  onRefreshChat,
  onQueueCountChange,
  onCancelableQueueCountChange,
  onDeliveryPendingChange,
  model,
  effort,
  onSelectionChange,
  ref,
}: AgentChatViewProps) {
  // What the agent is DOING, read alongside what it said. It polls only while a
  // turn runs, so an idle chat costs nothing.
  const activity = useAgentActivity(wsId, chatId, working, visible)
  const initialQueue = useMemo(() => loadPromptQueue(wsId, chatId), [wsId, chatId])
  const [queue, setQueue] = useState<PromptQueueItem[]>(initialQueue)
  const queueRef = useRef(initialQueue)
  const [persistenceLost, setPersistenceLost] = useState(false)
  const [draft, setDraft] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [composerError, setComposerError] = useState('')
  const [submitUnavailable, setSubmitUnavailable] = useState(false)
  const [showAwaitingTerminalHint, setShowAwaitingTerminalHint] = useState(false)

  useEffect(() => setSubmitUnavailable(false), [chatId, providerId])

  const [messages, setMessages] = useState<AgentChatMessage[]>([])
  const messagesRef = useRef<AgentChatMessage[]>([])
  const cursorRef = useRef(0)
  const oldestCursorRef = useRef(0)
  const [hasOlder, setHasOlder] = useState(false)
  const [messagesLoading, setMessagesLoading] = useState(true)
  const [messagesError, setMessagesError] = useState<Error | null>(null)
  const loadGeneration = useRef(0)
  const refreshInFlight = useRef(false)
  const refreshAgain = useRef(false)

  const [idleEpoch, setIdleEpoch] = useState(0)
  const idleEpochRef = useRef(0)
  const previousWorking = useRef(working)
  const previousTurnRevision = useRef(turnRevision)
  const dispatching = useRef('')

  const [catalogState, setCatalogState] = useState<CatalogState>({ state: 'closed' })
  const [slashOpen, setSlashOpen] = useState(false)
  const slashLeadingRef = useRef(false)
  const slashProviderRef = useRef(providerId)
  const [slashSelected, setSlashSelected] = useState(0)

  const updateQueue = useCallback(
    (change: (items: PromptQueueItem[]) => PromptQueueItem[]) => {
      const next = change(queueRef.current)
      queueRef.current = next
      setQueue(next)
      setPersistenceLost(!savePromptQueue(wsId, chatId, next))
      return next
    },
    [wsId, chatId],
  )

  useImperativeHandle(
    ref,
    () => ({
      cancelUnsentPrompts: () =>
        updateQueue((items) =>
          items.filter((item) => item.state !== 'queued' && item.state !== 'failed'),
        ),
    }),
    [updateQueue],
  )

  useEffect(() => onQueueCountChange?.(queue.length), [queue.length, onQueueCountChange])
  const cancelableQueueCount = queue.filter(
    (item) => item.state === 'queued' || item.state === 'failed',
  ).length
  useEffect(
    () => onCancelableQueueCountChange?.(cancelableQueueCount),
    [cancelableQueueCount, onCancelableQueueCountChange],
  )

  const deliveryPending = queue.some(
    (item) => item.state === 'submitting' || item.state === 'awaiting_turn',
  )
  useEffect(
    () => onDeliveryPendingChange?.(deliveryPending),
    [deliveryPending, onDeliveryPendingChange],
  )

  const awaitingHead = queue[0]?.state === 'awaiting_turn' ? queue[0] : undefined
  useEffect(() => {
    setShowAwaitingTerminalHint(false)
    if (!active || !visible || !live || working || !awaitingHead) return
    // The daemon has an answer, so this guess stays quiet: the terminal-wait
    // banner above is saying the same thing, without the "may".
    if (terminalWaiting) return

    const attemptedAt = Date.parse(awaitingHead.submittedAt ?? awaitingHead.createdAt)
    const delay = Math.max(0, AWAITING_TERMINAL_HINT_MS - (Date.now() - attemptedAt))
    if (delay === 0) {
      setShowAwaitingTerminalHint(true)
      return
    }
    const timer = window.setTimeout(() => setShowAwaitingTerminalHint(true), delay)
    return () => window.clearTimeout(timer)
  }, [active, visible, live, working, awaitingHead, terminalWaiting])

  useEffect(() => {
    if (previousWorking.current && !working) {
      idleEpochRef.current += 1
      setIdleEpoch(idleEpochRef.current)
    }
    previousWorking.current = working
  }, [working])

  const releaseBusyBarrier = useCallback((clientRequestId: string, epoch: number) => {
    const head = queueRef.current[0]
    if (
      head?.clientRequestId !== clientRequestId ||
      head.state !== 'queued' ||
      head.waitForIdleEpoch !== epoch ||
      epoch <= idleEpochRef.current
    )
      return
    idleEpochRef.current = epoch
    setIdleEpoch(epoch)
  }, [])

  // The stream's revision advances for every server-folded turn frame. React
  // may batch a fast busy -> idle pair so `working` renders false both before
  // and after it; the later revision plus a false folded value is nevertheless
  // an authoritative idle edge and must release a chat_busy retry barrier.
  useEffect(() => {
    const advanced = turnRevision > previousTurnRevision.current
    previousTurnRevision.current = turnRevision
    if (!advanced || working) return
    const head = queueRef.current[0]
    if (head?.state === 'queued' && head.waitForIdleEpoch !== undefined) {
      releaseBusyBarrier(head.clientRequestId, head.waitForIdleEpoch)
    }
  }, [turnRevision, working, releaseBusyBarrier])

  // A blocked FIFO also survives reloads and websocket outages. In those cases
  // there may be no future local edge to observe, so re-read the aggregate until
  // it reports idle. This never sends the prompt; it only lets the normal FIFO
  // dispatcher retry the same id after authoritative server confirmation.
  const busyHead =
    queue[0]?.state === 'queued' && queue[0].waitForIdleEpoch !== undefined ? queue[0] : undefined
  useEffect(() => {
    if (!busyHead || !active || !visible || !live) return
    let cancelled = false
    let checking = false
    const check = async () => {
      if (checking) return
      checking = true
      try {
        const serverWorking = await onRefreshChat()
        if (!cancelled && !serverWorking) {
          releaseBusyBarrier(busyHead.clientRequestId, busyHead.waitForIdleEpoch ?? 0)
        }
      } catch {
        // Stay blocked and try again. A failed read is not evidence of idle.
      } finally {
        checking = false
      }
    }
    void check()
    const timer = window.setInterval(() => void check(), BUSY_RECHECK_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [busyHead, active, visible, live, onRefreshChat, releaseBusyBarrier])

  const reconcileQueue = useCallback(
    (authoritative: AgentChatMessage[]) => {
      updateQueue((items) => {
        let changed = false
        const remaining = items.filter((item) => {
          if (!awaitingEvidence(item)) return true
          const confirmed = authoritative.some((message) => samePrompt(message, item))
          if (confirmed) changed = true
          return !confirmed
        })
        return changed ? remaining : items
      })
    },
    [updateQueue],
  )

  // A delivery the daemon has RETIRED resolves its own queue item, and nothing
  // else ever will: a provider built-in is handled inside the CLI, so no user
  // message for it is coming to the ledger and reconcileQueue above can never fire
  // on it. Without this the FIFO head sits in awaiting_turn and blocks the
  // composer for the rest of the runner's life.
  useEffect(() => {
    if (!settledPrompts?.length) return
    const settled = new Set(settledPrompts)
    updateQueue((items) => {
      const remaining = items.filter(
        (item) => !(awaitingEvidence(item) && settled.has(item.clientRequestId)),
      )
      return remaining.length === items.length ? items : remaining
    })
  }, [settledPrompts, updateQueue])

  const applyMessages = useCallback(
    (incoming: AgentChatMessage[]) => {
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      if (next.length > 0) {
        cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
        oldestCursorRef.current =
          oldestCursorRef.current === 0
            ? (next[0]?.sequence ?? 0)
            : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
      }
      reconcileQueue(next)
    },
    [reconcileQueue],
  )

  const loadInitialMessages = useCallback(async () => {
    const generation = ++loadGeneration.current
    setMessagesLoading(true)
    setMessagesError(null)
    try {
      const page = await listChatMessages(wsId, chatId, { limit: MESSAGE_PAGE_SIZE })
      if (generation !== loadGeneration.current) return
      messagesRef.current = []
      cursorRef.current = page.cursor
      oldestCursorRef.current = page.oldestCursor
      setHasOlder(page.hasMore)
      applyMessages(page.items)

      const evidencePending = () => queueRef.current.some(awaitingEvidence)
      if (!evidencePending()) return
      const baselines = queueRef.current
        .filter(awaitingEvidence)
        .map((item) => item.baselineSequence)
      const baseline = Math.min(...baselines)
      let exhaustedRecoveryBudget = false

      if (baseline > 0) {
        // Ask forward from the evidence boundary. This avoids walking a long
        // history backward when the confirming hook is old but its exact
        // baseline is already persisted with the queue item.
        let after = baseline
        for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
          const recovery = await listChatMessages(wsId, chatId, {
            after,
            limit: MESSAGE_PAGE_SIZE,
          })
          if (generation !== loadGeneration.current) return
          applyMessages(recovery.items)
          if (!evidencePending() || !recovery.hasMore) return
          if (recovery.cursor <= after) return
          after = recovery.cursor
          exhaustedRecoveryBudget = index === EVIDENCE_RECOVERY_MAX_PAGES - 1
        }
      } else if (page.hasMore && page.oldestCursor > 0) {
        // A brand-new chat has baseline 0, which the paging API represents as
        // "no after cursor". Walk older pages only for that special case.
        let before = page.oldestCursor
        for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
          const recovery = await listChatMessages(wsId, chatId, {
            before,
            limit: MESSAGE_PAGE_SIZE,
          })
          if (generation !== loadGeneration.current) return
          applyMessages(recovery.items)
          if (!evidencePending() || !recovery.hasMore) return
          if (recovery.oldestCursor <= 0 || recovery.oldestCursor >= before) return
          before = recovery.oldestCursor
          exhaustedRecoveryBudget = index === EVIDENCE_RECOVERY_MAX_PAGES - 1
        }
      }

      if (exhaustedRecoveryBudget && evidencePending()) {
        updateQueue((items) =>
          items.map((item) =>
            item.state === 'submitting' ||
            item.state === 'awaiting_turn' ||
            item.state === 'outcome_uncertain'
              ? {
                  ...item,
                  error:
                    item.error ??
                    'Prompt confirmation is older than the automatic recovery window. Load earlier messages or inspect Terminal before retrying.',
                }
              : item,
          ),
        )
      }
    } catch (error) {
      if (generation !== loadGeneration.current || isAbort(error)) return
      setMessagesError(error instanceof Error ? error : new Error(String(error)))
    } finally {
      if (generation === loadGeneration.current) setMessagesLoading(false)
    }
  }, [wsId, chatId, applyMessages, updateQueue])

  const refreshMessages = useCallback(async () => {
    if (refreshInFlight.current || messagesLoading) {
      refreshAgain.current = true
      return
    }
    refreshInFlight.current = true
    const generation = loadGeneration.current
    try {
      let after = cursorRef.current
      for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
        const page = await listChatMessages(wsId, chatId, {
          after,
          limit: MESSAGE_PAGE_SIZE,
        })
        if (generation !== loadGeneration.current) return
        applyMessages(page.items)
        setMessagesError(null)
        if (!page.hasMore) break

        // `hasMore` is only actionable when the server advanced the forward
        // cursor. A stale/malformed page must not create an unbounded promise
        // recursion that pegs the renderer while repeatedly fetching the same
        // messages. Empty pages are covered by the same cursor check.
        if (page.cursor <= after) break
        after = page.cursor
      }
    } catch (error) {
      if (!isAbort(error) && generation === loadGeneration.current) {
        setMessagesError(error instanceof Error ? error : new Error(String(error)))
      }
    } finally {
      refreshInFlight.current = false
      if (refreshAgain.current && generation === loadGeneration.current) {
        refreshAgain.current = false
        void refreshMessages()
      }
    }
  }, [wsId, chatId, messagesLoading, applyMessages])

  useEffect(() => {
    void loadInitialMessages()
    return () => {
      loadGeneration.current += 1
    }
  }, [loadInitialMessages])

  // A lifecycle frame changes the server-folded working value. Fetching on both
  // edges confirms the user and assistant hook messages. Poll while working (or
  // awaiting acceptance) because a turn_stop can legitimately leave Working true
  // while provider-reported async work continues.
  useEffect(() => {
    if (!visible) return
    void refreshMessages()
    const awaiting = queue.some(
      (item) => item.state === 'submitting' || item.state === 'awaiting_turn',
    )
    if (!working && !awaiting) return
    const timer = window.setInterval(() => void refreshMessages(), MESSAGE_POLL_MS)
    return () => window.clearInterval(timer)
  }, [working, turnRevision, providerId, visible, queue, refreshMessages])

  const loadOlder = async () => {
    const before = oldestCursorRef.current
    if (!before) return
    try {
      const page = await listChatMessages(wsId, chatId, {
        before,
        limit: MESSAGE_PAGE_SIZE,
      })
      applyMessages(page.items)
      oldestCursorRef.current = page.oldestCursor
      setHasOlder(page.hasMore)
    } catch (error) {
      setMessagesError(error instanceof Error ? error : new Error(String(error)))
    }
  }

  const mark = useCallback(
    (id: string, change: (item: PromptQueueItem) => PromptQueueItem) =>
      updateQueue((items) =>
        items.map((item) => (item.clientRequestId === id ? change(item) : item)),
      ),
    [updateQueue],
  )

  const dispatch = useCallback(
    async (item: PromptQueueItem) => {
      if (dispatching.current) return
      dispatching.current = item.clientRequestId
      onPromptDispatchStart?.()
      mark(item.clientRequestId, (current) => ({
        ...current,
        state: 'submitting',
        error: undefined,
        // A retry is the same logical delivery attempt. Moving this baseline
        // forward could hide late hook evidence from the first network call.
        baselineSequence:
          current.submittedAt === undefined ? cursorRef.current : current.baselineSequence,
        submittedAt: current.submittedAt ?? new Date().toISOString(),
        waitForIdleEpoch: undefined,
      }))
      try {
        const result = await submitAgentPrompt(wsId, chatId, item.text, item.clientRequestId)
        // Success means the replacement TUI exists, not that the provider has
        // accepted the message. Keep this row as the FIFO head until user_prompt
        // appears in the authoritative ledger.
        mark(item.clientRequestId, (current) => ({ ...current, state: 'awaiting_turn' }))
        try {
          await onPromptSpawned(result)
        } catch {
          // Runner adoption is view reconciliation. A successful prompt must never
          // be replayed because the follow-up GET/attach failed.
        }
        void refreshMessages()
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) {
          const code = error.code
          const definitelyBusy =
            code === 'chat_busy' || (!code && /\b(busy|working)\b/i.test(error.message))
          if (code === 'request_already_accepted') {
            // Dedupe proved the original restart succeeded. There is nothing to
            // resend and no need for a runner result: wait for hook evidence.
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'awaiting_turn',
              error: undefined,
            }))
            void refreshMessages()
          } else if (definitelyBusy) {
            const waitForIdleEpoch = idleEpochRef.current + 1
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'queued',
              waitForIdleEpoch,
              error:
                'The provider became busy before this prompt could start. It will wait for idle.',
            }))
          } else if (code === 'request_id_conflict') {
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'failed',
              error:
                'This request identity belongs to different prompt text. Edit the prompt to create a new request.',
            }))
          } else {
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'outcome_uncertain',
              error:
                error.message ||
                'Crowbar cannot prove whether the provider accepted this prompt. It was not retried.',
            }))
            void refreshMessages()
          }
        } else {
          const definitive =
            error instanceof ApiError && [400, 404, 422, 424].includes(error.status)
          if (
            error instanceof ApiError &&
            error.status === 422 &&
            error.code === 'prompt_submit_unsupported'
          )
            setSubmitUnavailable(true)
          mark(item.clientRequestId, (current) => ({
            ...current,
            // A rejected fetch or unexpected server failure can happen after
            // the daemon accepted and spawned. Only explicit pre-dispatch
            // statuses are safe to call failed; everything else blocks replay
            // until hook evidence or a human decision resolves it.
            state: definitive ? 'failed' : 'outcome_uncertain',
            error: errorMessage(error),
          }))
          if (!definitive) void refreshMessages()
        }
      } finally {
        dispatching.current = ''
        onPromptDispatchSettled?.()
      }
    },
    [
      wsId,
      chatId,
      mark,
      onPromptSpawned,
      onPromptDispatchStart,
      onPromptDispatchSettled,
      refreshMessages,
    ],
  )

  // Only the FIFO head can move. `awaiting_turn`, failed and uncertain heads are
  // deliberate barriers; later prompts cannot overtake them.
  useEffect(() => {
    const head = queue[0]
    if (!head || head.state !== 'queued' || working || !live || !active || !visible) return
    if (head.waitForIdleEpoch !== undefined && head.waitForIdleEpoch > idleEpoch) return
    void dispatch(head)
  }, [queue, working, live, active, visible, idleEpoch, dispatch])

  // A replacement CLI that disappears before user_prompt is not accepted. Keep
  // the same request identity but require a human retry; never silently resubmit.
  useEffect(() => {
    if (live) return
    const awaiting = queueRef.current.find(
      (item) => item.state === 'submitting' || item.state === 'awaiting_turn',
    )
    if (!awaiting) return
    mark(awaiting.clientRequestId, (item) => ({
      ...item,
      state: 'outcome_uncertain',
      error:
        'The provider exited before Crowbar observed prompt confirmation. Check the ledger or terminal before retrying.',
    }))
    void refreshMessages()
  }, [live, mark, refreshMessages])

  const enqueueDraft = () => {
    const text = draft.trim()
    if (!isPromptTextWithinLimit(text)) {
      setComposerError(
        text ? 'This prompt is too large to submit.' : 'Write a message before submitting.',
      )
      return
    }
    if (queueRef.current.length >= MAX_PROMPT_QUEUE_ITEMS) {
      setComposerError(`This chat already has ${MAX_PROMPT_QUEUE_ITEMS} pending prompts.`)
      return
    }
    let clientRequestId: string
    try {
      clientRequestId = requestId()
    } catch (error) {
      setComposerError(errorMessage(error))
      return
    }
    const item: PromptQueueItem = {
      clientRequestId,
      text,
      state: 'queued',
      createdAt: new Date().toISOString(),
      baselineSequence: cursorRef.current,
    }
    const next = [...queueRef.current, item]
    if (!canPersistPromptQueue(wsId, chatId, next)) {
      setComposerError(
        'Pending prompt storage is full or unavailable. Wait for queued work to finish before adding more.',
      )
      return
    }
    updateQueue(() => next)
    setDraft('')
    setComposerError('')
    slashLeadingRef.current = false
    setSlashOpen(false)
  }

  const editPrompt = (item: PromptQueueItem) => {
    updateQueue((items) =>
      items.filter((candidate) => candidate.clientRequestId !== item.clientRequestId),
    )
    setDraft(item.text)
    setComposerError('')
    window.requestAnimationFrame(() => textareaRef.current?.focus())
  }

  const cancelPrompt = (id: string) =>
    updateQueue((items) => items.filter((item) => item.clientRequestId !== id))

  const retryPrompt = (id: string) =>
    mark(id, (item) => ({
      ...item,
      state: 'queued',
      error: undefined,
      waitForIdleEpoch: working ? idleEpochRef.current + 1 : undefined,
      // SAME clientRequestId: server dedupe can return the first successful result.
    }))

  const updateDraft = (value: string) => {
    setDraft(value)
    setComposerError('')
    const leading = value.startsWith('/')
    if (leading && !slashLeadingRef.current) {
      setSlashOpen(true)
      setSlashSelected(0)
    } else if (!leading) {
      setSlashOpen(false)
    }
    slashLeadingRef.current = leading
  }

  // Provider/view changes close and abort the one-open-menu catalog. A leading
  // slash must be deleted/retyped to explicitly open a fresh provider probe.
  useEffect(() => {
    if (slashProviderRef.current !== providerId || !active) {
      slashProviderRef.current = providerId
      setSlashOpen(false)
      setCatalogState({ state: 'closed' })
    }
  }, [providerId, active])

  useEffect(() => {
    if (!slashOpen || !active) {
      setCatalogState({ state: 'closed' })
      return
    }
    const providerAtRequest = providerId
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setCatalogState({ state: 'loading' })
      void getSlashCatalog(wsId, chatId, controller.signal)
        .then((catalog) => {
          if (
            controller.signal.aborted ||
            providerAtRequest !== slashProviderRef.current ||
            catalog.providerId !== providerAtRequest
          )
            return
          setCatalogState({ state: 'ready', catalog })
          setSlashSelected(0)
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted || isAbort(error)) return
          setCatalogState({
            state: 'error',
            error: error instanceof Error ? error : new Error(String(error)),
            unavailable: error instanceof ApiError && error.status === 422,
          })
        })
    }, SLASH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [slashOpen, active, wsId, chatId, providerId])

  const slashQuery = draft.slice(1).trim().toLowerCase()
  const slashItems = useMemo(() => {
    if (catalogState.state !== 'ready') return []
    if (!slashQuery) return catalogState.catalog.items
    return catalogState.catalog.items.filter((item) =>
      `${item.label}\n${item.description}\n${item.source}\n${item.insertText}`
        .toLowerCase()
        .includes(slashQuery),
    )
  }, [catalogState, slashQuery])

  useEffect(() => {
    if (slashSelected >= slashItems.length) setSlashSelected(Math.max(0, slashItems.length - 1))
  }, [slashItems.length, slashSelected])

  // The one thing Enter and Tab both need: is there a completion to accept? An
  // open picker is not enough — it is open while the probe is still running, and
  // open over an empty result for every command the probe cannot see.
  const highlightedSlashItem = slashOpen ? slashItems[slashSelected] : undefined

  // The message being said right now, as a bubble below the recorded ones.
  //
  // Suppressed once the ledger has it. The two arrive from different places — the
  // live frame and the message poll — so for a moment both are present, and
  // rendering both would show the same sentence twice. Comparing the TEXT rather
  // than an id is what makes that work: the ledger row is keyed by the provider's
  // message id and this frame carries the same id, but the row is only written
  // when the message completes, so equal text is the earliest reliable signal
  // that the record has caught up.
  const streamingBubble = useMemo(() => {
    const text = streamingMessageText?.trim()
    if (!text || !streamingMessageId) return undefined
    if (messages.some((m) => m.role === 'assistant' && m.text.trim() === text)) return undefined
    return {
      sequence: Number.MAX_SAFE_INTEGER,
      role: 'assistant' as const,
      text: streamingMessageText ?? '',
      providerId,
      turnId: '',
      at: '',
    }
  }, [streamingMessageId, streamingMessageText, messages, providerId])

  const selectSlashItem = (item: SlashCatalogItem) => {
    setDraft(item.insertText)
    slashLeadingRef.current = item.insertText.startsWith('/')
    setSlashOpen(false)
    window.requestAnimationFrame(() => textareaRef.current?.focus())
  }

  const handleComposerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Escape' && slashOpen) {
      event.preventDefault()
      setSlashOpen(false)
      return
    }
    if (
      slashOpen &&
      slashItems.length > 0 &&
      (event.key === 'ArrowDown' || event.key === 'ArrowUp')
    ) {
      event.preventDefault()
      const direction = event.key === 'ArrowDown' ? 1 : -1
      setSlashSelected((selected) => (selected + direction + slashItems.length) % slashItems.length)
      return
    }
    // Tab is the completion key, and it is unconditional while something is
    // highlighted: it never sends, so it is the one keystroke a user can press
    // without first working out what the picker currently thinks it has.
    if (event.key === 'Tab' && !event.shiftKey && highlightedSlashItem) {
      event.preventDefault()
      selectSlashItem(highlightedSlashItem)
      return
    }
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault()
      // Enter accepts a completion when there IS one, and otherwise sends.
      //
      // It used to be swallowed for as long as the picker was open, on the
      // reasoning that Enter belonged to the picker and sending the query as a
      // prompt would spend tokens. That reasoning does not survive the catalog
      // being INCOMPLETE BY DECLARATION: no probe reports a provider's own
      // built-in commands, so /compact, /clear, /model and /context match nothing
      // and never will. With nothing to accept, the picker had no claim on the
      // key — and the composer went on saying "Enter to send" under a key that
      // did nothing at all, for exactly the commands a user reaches for most.
      //
      // A loading probe is the same case: the alternative is a composer that
      // silently ignores Enter for as long as a provider takes to answer, which
      // is up to ten seconds. Esc still closes the picker outright.
      if (highlightedSlashItem) {
        selectSlashItem(highlightedSlashItem)
        return
      }
      enqueueDraft()
    }
  }

  const previousAssistantProvider = (index: number): string => {
    for (let i = index - 1; i >= 0; i--) {
      if (messages[i]?.role === 'assistant') return messages[i]?.providerId ?? ''
    }
    return ''
  }

  return (
    <section className="flex h-full min-h-0 flex-col" aria-label="Agent chat">
      <div className="min-h-0 flex-1 overflow-y-auto px-1" data-testid="agent-message-list">
        <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col justify-end gap-5 py-5">
          {hasOlder && (
            <Button
              className="self-center"
              variant="ghost"
              size="sm"
              onClick={() => void loadOlder()}
            >
              Load earlier messages
            </Button>
          )}
          {messagesLoading && messages.length === 0 && (
            <div className="flex flex-1 items-center justify-center gap-2 text-muted-foreground text-sm">
              <FlickerSpinner className="size-4" /> Loading messages…
            </div>
          )}
          {messagesError && messages.length === 0 && (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
              <p>Couldn’t load this chat’s messages.</p>
              <div className="flex gap-2">
                <Button size="sm" variant="secondary" onClick={() => void loadInitialMessages()}>
                  Retry
                </Button>
                <Button size="sm" variant="ghost" onClick={onOpenTerminal}>
                  <TerminalIcon /> Terminal
                </Button>
              </div>
            </div>
          )}
          {!messagesLoading && !messagesError && messages.length === 0 && queue.length === 0 && (
            <div className="flex flex-1 flex-col items-center justify-center gap-1 text-center">
              <p className="font-medium text-sm">Start the conversation</p>
              <p className="text-muted-foreground text-xs">
                Messages appear here after the provider confirms them through hooks.
              </p>
            </div>
          )}
          {messages.map((message, index) => (
            <div key={message.sequence}>
              <MessageRow
                message={message}
                providers={providers}
                showProvider={
                  message.role === 'assistant' &&
                  (previousAssistantProvider(index) === '' ||
                    previousAssistantProvider(index) !== message.providerId)
                }
              />
              {message.role === 'assistant' && (
                <AgentTurnTools activity={activity} turnId={message.turnId ?? ''} />
              )}
            </div>
          ))}
          {streamingBubble && (
            <MessageRow
              message={streamingBubble}
              providers={providers}
              showProvider={false}
            />
          )}
          {queue.map((item) => (
            <QueueRow
              key={item.clientRequestId}
              item={item}
              showTerminalHint={
                showAwaitingTerminalHint && item.clientRequestId === awaitingHead?.clientRequestId
              }
              onEdit={() => editPrompt(item)}
              onCancel={() => cancelPrompt(item.clientRequestId)}
              onRetry={() => retryPrompt(item.clientRequestId)}
              onOpenTerminal={onOpenTerminal}
            />
          ))}
          {/* What the agent is BLOCKED ON, above what it is doing. The strip goes
              quiet while one of these is open: a chat waiting on a person is not
              working, and the prompt says everything the interruption banner did
              plus what to do about it. */}
          <AgentChoicePrompts
            wsId={wsId}
            chatId={chatId}
            activity={activity}
            providerLabel={providerName(providers, providerId)}
            onOpenTerminal={onOpenTerminal}
          />
          <AgentActivityStrip
            activity={activity}
            working={working}
            providerLabel={providerName(providers, providerId)}
          />
        </div>
      </div>

      <div className="mx-auto w-full max-w-3xl shrink-0 pb-3">
        {/* What this chat will RUN as on the left, what it has SPENT on the right.
            Both are provider-declared and both render nothing at all when the
            provider reports nothing, so the row collapses to whichever half exists
            — the picker's wrapper keeps the gauge on its column when it does not. */}
        <div className="mb-1 flex items-center justify-between gap-3">
          <div className="flex min-w-0">
            <AgentModelPicker
              wsId={wsId}
              chatId={chatId}
              provider={providers.find((provider) => provider.id === providerId)}
              model={model}
              effort={effort}
              onSelectionChange={onSelectionChange}
            />
          </div>
          <AgentContextGauge wsId={wsId} chatId={chatId} visible={visible} />
        </div>
        {persistenceLost && (
          <p className="mb-2 text-warning-foreground text-xs" role="alert">
            Pending prompts cannot be saved on this device. Keep Crowbar open until they finish.
          </p>
        )}
        {submitUnavailable || !live ? (
          <div className="flex items-center justify-between gap-3 rounded-xl border bg-muted/30 px-4 py-3 text-sm">
            <span className="text-muted-foreground">
              {live
                ? 'This provider does not support React prompt submission.'
                : 'Resume the provider before sending from Chat.'}
            </span>
            <Button size="sm" variant="secondary" onClick={onOpenTerminal}>
              <TerminalIcon /> Terminal
            </Button>
          </div>
        ) : (
          <div className="relative">
            {slashOpen && (
              <SlashPicker
                state={catalogState}
                items={slashItems}
                selected={slashSelected}
                onSelect={selectSlashItem}
                onOpenTerminal={onOpenTerminal}
              />
            )}
            <div className="rounded-2xl border bg-background p-1.5 shadow-xs focus-within:ring-2 focus-within:ring-ring/30">
              <Textarea
                ref={textareaRef}
                unstyled
                className="border-0 bg-transparent shadow-none before:hidden"
                aria-label="Message the agent"
                aria-expanded={slashOpen}
                aria-controls={slashOpen ? 'agent-skill-picker' : undefined}
                value={draft}
                placeholder={working ? 'Queue a message…' : 'Message the agent…'}
                onChange={(event) => updateDraft(event.target.value)}
                onKeyDown={handleComposerKeyDown}
              />
              <div className="flex items-center justify-between gap-2 px-1 pb-1">
                <span className="text-muted-foreground text-xs">
                  {highlightedSlashItem
                    ? 'Enter or Tab to insert · Esc to dismiss'
                    : 'Enter to send · Shift+Enter for newline'}
                </span>
                <Button
                  size="icon-sm"
                  variant="default"
                  aria-label={working || queue.length > 0 ? 'Queue prompt' : 'Send prompt'}
                  disabled={!draft.trim()}
                  onClick={enqueueDraft}
                >
                  <ArrowUpIcon />
                </Button>
              </div>
            </div>
          </div>
        )}
        {composerError && (
          <p className="mt-1.5 px-1 text-destructive text-xs" role="alert">
            {composerError}
          </p>
        )}
      </div>
    </section>
  )
}

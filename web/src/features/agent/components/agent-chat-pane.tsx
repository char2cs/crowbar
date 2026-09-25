import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { TrashIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'
import {
  getChat,
  switchProvider,
  switchToNative,
  switchToTerminal,
} from '@/features/agent/api/agent-api'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { AGENT_CYCLE_PROVIDER, AGENT_TOGGLE_VIEW_MODE } from '@/features/keymaps/registry'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { usePaneSession } from '@/features/agent/hooks/use-pane-session'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import {
  SPLIT_MIN_HALF_PX,
  SPLIT_MIN_STACKED_PX,
  useChatPresentation,
  useTerminalGridSlack,
} from '@/features/agent/hooks/use-chat-presentation'
import { PaneSash } from '@/features/panes/components/pane-sash'
import { cn } from '@/lib/utils'
import { IS_MAC } from '@/utils/platform'
import {
  AgentReturnToChatNotice,
  AgentTerminalWaitBanner,
} from '@/features/agent/components/agent-terminal-wait-banner'
import { AgentChatView, type AgentChatViewHandle } from '@/features/agent/chat/agent-chat-view'
import { AgentTerminalSurface } from '@/features/agent/terminal/agent-terminal-surface'

// ── The split view's geometry ─────────────────────────────────────────────
//
// A CLI TUI IS A FIXED-COLUMN GRID, and that is the only constraint here worth
// spelling out. Squeeze a terminal and it does not scroll — it REFLOWS: box
// drawing breaks apart, lines wrap mid-word, and the ground truth you opened the
// split to read the chat AGAINST stops being readable. Every number below is one
// answer to that, measured in the running app at the default terminal metrics
// (~8.7px per column):
//
//   the DEFAULT gives the terminal the larger half, because the chat re-wraps to
//     whatever it is handed and the TUI does not. On a full-width pane that lands
//     the TUI at ~82 columns — a conventional screen, and the width the CLIs
//     themselves lay out for.
//   the FLOOR is a floor, not a working width: it stops a drag collapsing either
//     half to a sliver you then cannot grab. A terminal held at it is narrow, and
//     is meant to read as "you have dragged this too far".
//   BELOW the side-by-side threshold there is no ratio that leaves the TUI usable,
//     so the split STACKS instead of shrinking — a short terminal at the FULL pane
//     width still wraps the way the CLI intended, where a tall narrow one does not.

interface AgentChatPaneProps {
  /** The chat this pane is pointed at. NOT stable for its life: the pane re-points it. */
  chatId: string
  /** The runner this pane follows, or '' when it has none (a dormant chat). */
  runnerId: string
  wsId: string
  /** The `PaneGroup` this surface renders inside. */
  paneId: string
  isActivePane: boolean
  /**
   * Whether this tab is the ACTIVE (visible) tab in its pane — distinct from
   * isActivePane (whether the pane has focus). The pane keeps every chat mounted
   * `visibility:hidden` for keep-alive, so a chat can be MOUNTED-BUT-HIDDEN; an
   * attached chat stays attached while hidden.
   */
  isVisible: boolean
  /**
   * Does an overlay chat header (ChatColumnHeader / ChatOnlyPaneHeader,
   * pane-top-row.tsx's `variant="chat-blur" overlay`) float above this pane
   * right now? That header paints no fill of its own and reserves no flex
   * space — it is `position: absolute; top: 0`, so nothing about this pane's
   * own layout otherwise knows it is there. It still owns a REAL, clickable
   * row (44px Mac / 34px elsewhere — PaneTopRow.ROW_HEIGHT_PX), so every
   * pinned-near-top surface here (the reviving/idle banners below, the
   * terminal-wait banner) has to clear THAT, or its own controls render
   * underneath the header's hit-box instead of in front of it. Pane-
   * container's own answer: true for chatFillsPane/chatVisibleAlongsideEditor,
   * false for the collapsed 'tabs' presentation's small in-flow
   * ChatBranchHeader, which already reserves its own space.
   */
  belowOverlayHeader?: boolean
}

// PaneTopRow's own real, clickable row height (see that file's ROW_HEIGHT_PX
// and its own doc) — this pane has no import of that component to share a
// constant with (features/panes/utils/pane-border.ts and
// components/layout/sidebar-peek.tsx each keep their own copy of the same
// platform fact rather than reach across features for it).
const HEADER_ROW_HEIGHT_PX = IS_MAC ? 44 : 34

// Mirrors features/tabs/components/pane-top-row.tsx's CHAT_BLUR_EXTRA_PX
// (56) added to this file's own HEADER_ROW_HEIGHT_PX — the FULL vertical
// reach of the header's EdgeDissolve backdrop-filter gradient, not just
// its clickable row. Text resting inside this zone but outside the
// narrower headerClearancePx below still renders visibly blurred (some of
// EdgeDissolve's mask layers carry blur(16px)-blur(64px) well past the row
// height) even though it is not covered/unclickable.
const CHAT_BLUR_ZONE_PX = HEADER_ROW_HEIGHT_PX + 56

// A flat, opaque pane: one centred column holding the live agent terminal, with the
// provider-switch dropdown beneath it on the same column. See the render for why this
// is NOT a card.
//
// THE TAB IS A VIEWPORT ON A MOVING TARGET. What a chat pane shows is not a chat, it is
// a RUNNER — the vendor-CLI process — and that process moves:
//
//   the runner MOVES to another chat (the user types /clear or /resume INSIDE the CLI,
//     and it switches conversation). The pane follows it: chatId is re-pointed, the tab
//     relabels — and because the terminal is keyed by the runner's PTY, which a move
//     does not change, XTERM NEVER REMOUNTS. The conversation changes without the
//     terminal changing. Pinning the pane to a chatId is what produced the bug this
//     replaces: the tab said "this agent has exited" and offered a Resume button that
//     spawned a SECOND CLI, while the first was alive and well in a chat the user then
//     had to go and find.
//
//   the runner is REPLACED on the same chat (a provider switch, or a Resume of a dormant
//     chat). The pane adopts the runner now on its chat: runnerId is re-pointed, and
//     since that runner has a PTY of its own, the terminal re-attaches.
//
// Both are one rule, applied in order: FOLLOW MY RUNNER IF IT STILL EXISTS ANYWHERE;
// OTHERWISE ADOPT WHOEVER IS ON MY CHAT. Attaching, relabelling and the exited state all
// fall out of it rather than being engineered separately.
// react-doctor-disable-next-line no-giant-component -- accepted: cohesive pane — drives a single live PTY-backed chat with tightly-coupled resize/focus/stream effects; no independent sub-surface to lift out.
export function AgentChatPane({
  chatId,
  runnerId,
  wsId,
  paneId,
  isActivePane,
  isVisible,
  belowOverlayHeader = false,
}: AgentChatPaneProps) {
  const store = useWorkspaceStore()
  // Extra top clearance every pinned-near-top surface below needs to clear the
  // overlay header's own real click target, plus that surface's original
  // breathing room (8px — the `top-2`/`mt-2` each one used to carry on its own).
  const headerClearancePx = belowOverlayHeader ? HEADER_ROW_HEIGHT_PX + 8 : 8
  // The TRANSCRIPT's own, larger clearance: the header's full EdgeDissolve
  // zone (CHAT_BLUR_ZONE_PX above), not just its click target. Distinct from
  // headerClearancePx above — that number stays correct for the opaque
  // reviving/idle/terminal-wait banners and for composer/empty-document's own
  // clearance (unaffected by this), which only need to clear the header's
  // hit-box, not the full reach of its blur gradient.
  const transcriptHeaderClearancePx = belowOverlayHeader ? CHAT_BLUR_ZONE_PX : 0

  // Where is MY runner? '' when it is nowhere — it exited, or a switch replaced it. A
  // chat is live exactly while a runner is placed on it, so this lookup is also what
  // makes a MOVE visible: the runner turns up under a different chat id.
  const runnerChatId = useStore(store, (s) =>
    runnerId ? (s.agentChats.chats.find((c) => c.liveRunnerId === runnerId)?.id ?? '') : '',
  )

  // The chat this pane is SHOWING: my runner's chat if it still has one (it moved, and
  // the tab follows), else the chat the tab was pointed at (which may be dormant).
  const shownChatId = runnerChatId || chatId

  // Has an authoritative list ever landed? That is what turns `!known` from
  // "not yet" into "not in it" — see the resolve effect below.
  const listSeeded = useStore(store, (s) => s.agentChats.listSeeded)
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.activeProviderId ?? '',
  )
  // The surface this chat is on RIGHT NOW (design spec 2.5, domain.Chat.Surface
  // — birth seeds it and the switch calls move it). Two things read it:
  //
  //   enterTerminal — 'terminal' means the daemon has no api connection for
  //     this chat, so there is no native view left to ask for.
  //   the compaction control — Crowbar offers it on its own chat only, and
  //     the daemon refuses one issued from the provider's terminal.
  const onTerminalSurface = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.surface === 'terminal',
  )
  const working = useStore(store, (s) => s.agentChats.working[shownChatId] ?? false)
  // Live mid-compaction, from the direct WS push — never derived from
  // `activity`. See AgentChatsState.compacting's own doc comment for why.
  const compacting = useStore(store, (s) => s.agentChats.compacting[shownChatId] ?? false)
  const turnRevision = useStore(store, (s) => s.agentChats.turnRevision[shownChatId] ?? 0)
  const title = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.title ?? '',
  )
  // The chat's sticky selection, '' when it has made none. Two narrow selectors
  // rather than the chat object: a selector returning the row itself would re-run
  // every consumer on any field of it.
  const chatModel = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.model ?? '',
  )
  const chatEffort = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.effort ?? '',
  )
  // What the live runner actually spawned MODEL as — absent on a dormant
  // chat. Never fed into stagedSelection/effective*: that pair is the NEXT
  // launch's request, this is the CURRENT one's already-resolved fact.
  // Effort has no equivalent read here: AgentChatView derives its displayed
  // effort per-turn off the ledger instead (see its own `latestTurnEffort`).
  const launchModel = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.launchModel ?? '',
  )
  // THE MACHINE-LEVEL LIST WHEN THIS WORKSPACE HAS NO COPY OF ITS OWN.
  //
  // `agentChats.providers` is seeded by use-workspace-agent-chats-stream, which
  // only runs while that workspace is MOUNTED. A pane outlives its workspace's
  // visibility by design (WorkspaceView's own doc), and
  // getOrCreateWorkspaceStore happily mints an empty store for a workspace
  // nothing has opened — so reopening an already-run conversation could land
  // here with `[]`. Every provider control is absence-not-disabled, so an empty
  // catalogue does not grey the picker out, it DELETES it
  // (agent-selection-picker's own `catalogueProviders.length === 0` guard):
  // the chat renders with no provider named and no control to name one with,
  // recoverable only by toggling a provider in Settings, whose write path
  // repairs this copy as a side effect. Same reasoning space-content-actions.ts
  // already reached for the New-thread button.
  const wsProviders = useStore(store, (s) => s.agentChats.providers)
  const globalProviders = useAgentProvidersStore((s) => s.providers)
  const providers = wsProviders.length > 0 ? wsProviders : globalProviders
  // Neither copy populated: this pane is the first surface to need the list, so
  // it asks. The store owns the read/write generation rules, so a load issued
  // here cannot clobber a preferences write it races.
  useEffect(() => {
    if (wsProviders.length > 0 || globalProviders.length > 0) return
    void useAgentProvidersStore.getState().load(wsId)
  }, [wsProviders.length, globalProviders.length, wsId])

  // Is this chat's CLI parked on a prompt Crowbar CANNOT answer — a workspace
  // trust dialog, a first-run screen, a login — which reaches the daemon through
  // no hook and would otherwise render as nothing at all?
  //
  // A PRIMITIVE selector on purpose. `undefined` means nothing is blocking this
  // chat; '' means it is blocked and the daemon could not identify by what; a
  // non-empty string names the prompt. Selecting the object would re-run this on
  // every reseed that rebuilt it, for a value that had not changed.
  const waitKind = useStore(store, (s) => s.agentChats.terminalWaits[shownChatId]?.kind)
  const waiting = waitKind !== undefined

  // Prompts the daemon has reported as delivered-and-over without a turn. The
  // composer resolves a pending item when its text turns up in the ledger, and for
  // a provider built-in it never does — see AgentChatsState.settledPrompts.
  const settledPrompts = useStore(store, (s) => s.agentChats.settledPrompts[shownChatId])

  // And the ones it retired with no proof of anything. Kept apart because the
  // composer must NOT discard these — their text is the last copy of what the
  // user typed. See AgentChatsState.abandonedPrompts.
  const abandonedPrompts = useStore(store, (s) => s.agentChats.abandonedPrompts[shownChatId])

  // The message(s) the agent is mid-way through saying — an array because a
  // turn can have more than one open item (Codex; Claude is always 0-or-1).
  // One selector, not per-field primitives: this is an Immer store, so the
  // array reference itself only changes when an entry's content actually
  // does (structural sharing — including Immer's own no-op detection when an
  // upsert writes byte-identical text, e.g. a resent frame), and narrowed to
  // this ONE chat's slot so another chat's streaming update never reaches it.
  const streamingMessages = useStore(store, (s) => s.agentChats.streamingMessages[shownChatId])
  // The agent's in-flight thinking. Live-only and never recorded, which is why it
  // is its own slot and not part of streamingMessages — see the slice's own doc.
  const reasoning = useStore(store, (s) => s.agentChats.streamingReasoning[shownChatId]?.text)
  const toolOutput = useStore(store, (s) => s.agentChats.streamingToolOutput[shownChatId])
  const plan = useStore(store, (s) => s.agentChats.streamingPlan[shownChatId])

  const columnRef = useRef<HTMLDivElement>(null)
  const splitContainerRef = useRef<HTMLDivElement>(null)
  const {
    presentation,
    setPresentation,
    splitEnabled,
    splitting,
    returnOffered,
    setReturnOffered,
    splitFocus,
    setSplitFocus,
    splitSizes,
    setSplitSizes,
    splitStacked,
  } = useChatPresentation(shownChatId, splitContainerRef)
  const [queuedPromptCount, setQueuedPromptCount] = useState(0)
  const [cancelablePromptCount, setCancelablePromptCount] = useState(0)
  const [promptReplacing, setPromptReplacing] = useState(false)
  const [deliveryPending, setDeliveryPending] = useState(false)
  // Starts true (fail open): until the view reports otherwise, the safer guess
  // is "the composer may not exist yet" — the cost of a stray frame of this
  // pane's own reviving/idle banner sitting beside a composer that turns out to
  // exist too is nothing; the cost of the other guess is the gap this exists
  // to close, silently, on a brand-new chat.
  const [chatBlank, setChatBlank] = useState(true)
  const chatViewRef = useRef<AgentChatViewHandle>(null)
  // CSS `zoom`, not transform: scale — it relayouts the surface instead of just
  // repainting it, and it's scoped to the chat surface only (the terminal has
  // its own font-size-based terminalZoomLevel).
  const chatZoom = useZoomStore.use.zoom()
  const providerName = providers.find((p) => p.id === activeProviderId)?.displayName || 'the agent'
  const { known, liveRunnerId, attachment, revival, sessionNote, canSend, startSession } =
    usePaneSession({
      store,
      wsId,
      chatId: shownChatId,
      providerName,
      presentation,
      promptReplacing,
    })

  // Whether each blank-chat signpost below is ABOUT to occupy
  // AgentEmptyDocument's own control-bar slot this pass. At most one renders
  // (waiting requires a live runner; reviving requires none).
  const waitingBannerShown = waiting && chatBlank
  const revivingBannerShown =
    presentation !== 'terminal' && chatBlank && attachment.state === 'reviving' && !promptReplacing

  // The two layout divs whose empty space belongs to the terminal, and the terminal's
  // own imperative handle — see focusTerminalFromEmptySpace.
  const rootRef = useRef<HTMLDivElement>(null)
  const terminalApiRef = useRef<{ focus: () => void } | null>(null)

  // The split's own three boxes: the container that defines 100%, and the two
  // halves PaneSash mutates imperatively during a drag. They are the SAME divs
  // that hold each surface in the other two modes — one element per surface,
  // whichever mode is on — so nothing about chat or terminal changes shape when
  // the diagnostic is off.
  const chatSurfaceRef = useRef<HTMLDivElement>(null)
  const terminalSurfaceRef = useRef<HTMLDivElement>(null)

  const gridSlack = useTerminalGridSlack(
    columnRef,
    attachment.state === 'attached' && presentation === 'terminal',
  )

  // Keep the pane's runner in step with its OWN chat. Following the runner
  // onto a different chat is the stream's `retargetPane` alone — it also runs
  // when no pane is mounted.
  useEffect(() => {
    if (!known || shownChatId !== chatId) return
    if (liveRunnerId === runnerId) return
    windowPaneStore.getState().paneActions.setPaneRunner(paneId, liveRunnerId || null)
  }, [paneId, known, chatId, runnerId, shownChatId, liveRunnerId])

  // NO TAB RELABEL HERE ANY MORE. A chat used to be a buffer whose `name` was the tab
  // label, so a title arriving after the tab opened (the agent auto-titles the chat via
  // WS `title_set`, the user renames it from the sidebar, or the runner MOVED us to a
  // fresh chat with a title of its own) had to be mirrored onto that buffer. Task 17's
  // `ChatHead` reads `agentChats.chats.find(c => c.id === chatId)?.title` straight from
  // the store instead, so all three cases are live for free — and the third one now
  // works, because the effect above re-points the PANE's own chatId rather than a
  // buffer that has not existed since Task 1.

  // Is this pane still on screen? A read can land after it is gone, and applying
  // one then writes into a torn-down store. Set on every mount so StrictMode's
  // double-effect does not leave it false for the run that survives.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // Re-check the aggregate after a prompt race. The prompt queue consumes only
  // this server-folded value; it never guesses busy state from a lifecycle kind.
  // Applied under the version rule like every other chat read.
  const refreshChatWorking = useCallback(async (): Promise<boolean> => {
    const fetched = await getChat(wsId, shownChatId)
    const s = store.getState()
    if (mountedRef.current) s.applyAgentChat(fetched)
    return store.getState().agentChats.working[fetched.id] === true
  }, [store, wsId, shownChatId])

  // A pick the picker has made but NOT sent yet — provider included. The
  // picker itself never calls the selection API or SwitchProvider any more —
  // it only ever updates this, pure local state — so choosing a row never
  // mutates the chat or touches its CLI until the user actually sends
  // (submitAgentPrompt commits it atomically with the prompt). null means
  // "nothing staged, show the chat's own current provider / sticky value."
  const [stagedSelection, setStagedSelection] = useState<{
    providerId: string
    model: string
    effort: string
  } | null>(null)

  // The staged override settles the moment the store's OWN provider and
  // sticky value catch up to it — derived at render time, not cleared by an
  // effect a tick later, so there is no render where a just-landed store
  // update is briefly masked by a stale staged copy. Once settled,
  // stagedSelection's own fields equal the live store's, so reading through
  // it (`stagedSelection?.x ?? live`) is indistinguishable from having
  // cleared it; a moment-later store update from a DIFFERENT source (another
  // pane on the same chat, a stale refetch) still wins over a staged copy
  // exactly as it would if nothing had ever been staged, because it is
  // compared fresh on every render rather than trusted from whenever it was
  // set. Provider settles via the SAME adopt() refresh a plain prompt
  // already triggers (handlePromptSpawned) — no separate commit call needed
  // for it, unlike model/effort below.
  const effectiveProviderId = stagedSelection?.providerId ?? activeProviderId
  const effectiveModel = stagedSelection?.model ?? chatModel
  const effectiveEffort = stagedSelection?.effort ?? chatEffort

  // The picker's own pick. Local only — see stagedSelection above.
  const stageSelection = useCallback((providerId: string, model: string, effort: string) => {
    setStagedSelection({ providerId, model, effort })
  }, [])

  // A staged pick the SERVER just accepted, via the next prompt it rode
  // along with (usePromptQueue's onSelectionCommitted). The store is the
  // chat's owner for the same reason it always was: nothing else brings this
  // pair back, since submitAgentPrompt's response carries no body either.
  const commitSelection = useCallback(
    (model: string, effort: string) => {
      store.getState().setAgentChatSelection(shownChatId, model, effort)
    },
    [store, shownChatId],
  )

  // Bounds streamingMessages[shownChatId]: once useChatMessages reports an id
  // the ledger now confirms for real, its store-side entry is dead weight —
  // see pruneAgentChatStreamingMessages' own doc comment for why this is safe
  // where clearing the whole array on a turn boundary was not.
  const handleStreamingSettled = useCallback(
    (ids: string[]) => {
      store.getState().pruneAgentChatStreamingMessages(shownChatId, ids)
    },
    [store, shownChatId],
  )

  // A CHAT THE LIST NEVER MENTIONS.
  //
  // `known` is this pane's entire basis for "do we know what this chat is", and
  // while it is false `attachment` reads `pending` — which renders nothing,
  // spawns nothing and asks nothing. That is exactly right for the moment before
  // the list lands, and exactly wrong once it has: a pane pointed at a chat the
  // list does not carry is then waiting on a fact that is never coming. A
  // restored layout, a chat opened from another scope, or a list that raced the
  // pane all land here.
  //
  // And the wait is SELF-SEALING, which is what makes it permanent rather than
  // merely wrong. Every path that could teach the store this chat exists —
  // adopt(), refreshChatWorking() — is reachable only through code gated on
  // `live`, and `live` is gated on this. Nothing breaks the cycle from inside
  // it. The visible cost is the prompt queue: its dispatcher bails on `!live`
  // every pass, so the composer holds the user's message on "queued" forever and
  // never attempts the POST — no error, no retry, no request at all.
  //
  // So ask the daemon, ONCE per chat per mount. Same budget shape as the revive
  // budget above and for the same reason — a chat that is genuinely gone must
  // not become a retry storm — and like adopt() this only ever READS. Spawning
  // and attaching stay the attach effect's job, which takes over the moment the
  // row lands in the store.
  const resolvedRef = useRef(new Set<string>())
  useEffect(() => {
    if (!listSeeded || known || !shownChatId) return
    if (resolvedRef.current.has(shownChatId)) return
    resolvedRef.current.add(shownChatId)
    void getChat(wsId, shownChatId)
      .then((chat) => {
        store.getState().applyAgentChat(chat)
      })
      .catch(() => {
        // Genuinely gone, or the read failed. The pane has asked its one
        // question; `pending` is now an honest "nothing to show" instead of a
        // wait, and the user drives from the sidebar.
      })
  }, [store, wsId, listSeeded, known, shownChatId])

  // Switch the provider ON THE CHAT THE RUNNER IS IN NOW (shownChatId — after a
  // /clear the pane shows a different conversation than the one it opened on).
  // The daemon owns the lifecycle: its `switching` phase is the spinner, its
  // snapshot frames re-point the pane. This only reports a refusal.
  //
  // AWAITABLE, and its answer is load-bearing: picking another provider's model
  // from the identity chip is two writes, and the second is only legal once the
  // first has landed.
  const [switchInFlight, setSwitchInFlight] = useState(false)
  const handleSwitch = async (providerId: string): Promise<boolean> => {
    if (switchInFlight || promptReplacing || deliveryPending || attachment.state === 'reviving')
      return false
    const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
    setSwitchInFlight(true)
    try {
      await switchProvider(wsId, shownChatId, providerId)
      // A switch never toggles `presentation`, so a chat already on the terminal
      // surface stays there — and only enterTerminal knows whether THIS provider
      // needs a native view forked for it.
      if (presentation === 'terminal') enterTerminal(providerId)
      return true
    } catch (err: unknown) {
      toastSpawnFailure(err, name, 'switch to')
      return false
    } finally {
      setSwitchInFlight(false)
    }
  }
  // ⌘/ cycles this chat to the NEXT ENABLED provider, the way ⌘-tab cycles apps.
  //
  // It lives here rather than in usePaneKeyboard because switching is not just an
  // API call: handleSwitch owns the transient-dormancy guard, the "Starting …"
  // attachment state and the 424-aware toast. Re-implementing that in the global
  // dispatcher would fork the switch flow.
  //
  // THE LISTENER IS ON `window`, SO IT IS NOT SCOPED BY ANYTHING IT RENDERS
  // INSIDE. Three separate things keep a chat mounted while the user is looking
  // somewhere else, and each needs its own answer here:
  //
  //   another PANE has focus         → isActivePane
  //   another TAB is showing in this pane (chats stay mounted for keep-alive)
  //                                  → isVisible
  //   another WORKSPACE is in view   → the wsId check inside onKeyDown
  //
  // The third one is the reason this is not "by construction". A retained
  // workspace stays MOUNTED under `display:none` + `inert` (workspace-host), and
  // neither hides a window-level listener nor changes this effect's deps — so N
  // retained workspaces each satisfied isActivePane && isVisible at once. ⌘/
  // pressed in workspace B was swallowed here by A's invisible listener: the
  // preventDefault killed B's Monaco comment toggle, and A switched provider on a
  // chat the user could not see, killing that CLI and spawning another.
  //
  // It is asked INSIDE the handler rather than in the guard because the active
  // workspace changes without re-rendering this pane — a dep on it would leave
  // the listener registered against a stale answer, which is the bug itself.
  // The chord comes from the keymap so it stays rebindable.
  const cycleChord = useEffectiveChordMap()[AGENT_CYCLE_PROVIDER]
  const onCycleProvider = useEffectEvent(() => {
    const enabled = providers.filter((p) => p.enabled)
    if (enabled.length < 2) return
    const i = enabled.findIndex((p) => p.id === activeProviderId)
    // An unknown current provider (a chat on one since disabled) starts the cycle
    // at the top of the list rather than doing nothing.
    const next = enabled[(i + 1) % enabled.length]
    if (!next || next.id === activeProviderId) return
    void handleSwitch(next.id)
  })

  useEffect(() => {
    if (!isActivePane || !isVisible || !cycleChord) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (!eventMatchesChord(e, cycleChord)) return
      // A retained (hidden) workspace's pane must neither act NOR swallow the key.
      if (getActiveWorkspaceId() !== wsId) return
      e.preventDefault()
      e.stopPropagation()
      onCycleProvider()
    }
    // CAPTURE phase, and that is the whole point. When a chat is open the focus is
    // in its xterm, which preventDefaults + stopPropagations the keys it handles —
    // so a bubble-phase listener never sees the chord in the one place this command
    // is meant to work. Every other chord that must win over a focused terminal
    // registers the same way (use-workspace-switcher-keyboard, hover-tooltip).
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [isActivePane, isVisible, cycleChord, wsId])

  // ── Escorting the user to the terminal, and back ──────────────────────────
  //
  // escortRef records whether CROWBAR put the user in front of the terminal, and
  // whether it still owns putting them back:
  //
  //   'none'     — we did not move them. We will not move them back either: a
  //                user who walked to the terminal themselves is not lost.
  //   'sent'     — we moved them and they have not touched the switcher since, so
  //                the surface they are on is still OUR choice to undo.
  //   'released' — they are here because of us, but they have since chosen a
  //                surface themselves. From then on their choice outranks ours:
  //                we OFFER the way back and never take it.
  //
  // A ref rather than state because nothing renders from it — it decides what an
  // edge does, and a render in between would only be a chance for the two to
  // disagree.
  const escortRef = useRef<'none' | 'sent' | 'released'>('none')

  // Is the user actually looking at THIS chat right now?
  //
  // Three independent things keep a chat mounted while they are somewhere else,
  // and all three have to say yes before this pane may navigate on its own:
  // another pane has focus (isActivePane), another tab is showing in this pane
  // (isVisible), or another workspace is in view — which is invisible from here,
  // because a retained workspace stays MOUNTED under display:none + inert. See
  // the ⌘/ handler above for what happened the last time that third axis was
  // assumed rather than asked.
  const userIsWatching = useEffectEvent(
    () => isActivePane && isVisible && getActiveWorkspaceId() === wsId,
  )

  // Flips presentation to 'terminal'. A hotswap provider's PTY is live from the
  // spawn, and a chat already on the terminal surface is its own PTY, so only
  // a chat whose provider talks over its api connection asks the daemon to
  // move it (switchToTerminal) — which hands the session over or relaunches
  // the TUI on the resume ladder. Every path onto the terminal goes through
  // here (the escort below, the wait banner, the composer's link). A plain
  // function: it is called from effect events and click handlers alike.
  // providerIdOverride: handleSwitch calls this right after switchProvider
  // resolves, before a re-render has caught activeProviderId up.
  const enterTerminal = (providerIdOverride?: string) => {
    const chatProvider = providers.find((p) => p.id === (providerIdOverride ?? activeProviderId))
    const hotswap = chatProvider ? chatProvider.hotswap === true : true
    if (hotswap || onTerminalSurface) {
      setPresentation('terminal')
      return
    }
    void (async () => {
      try {
        await switchToTerminal(wsId, shownChatId)
        setPresentation('terminal')
      } catch (err: unknown) {
        // Left where they were: a view that never came up is worse than
        // staying put, and a refusal (a turn in flight) must still be said.
        const name = chatProvider?.displayName ?? providerIdOverride ?? activeProviderId
        toastSpawnFailure(err, name, 'open the terminal view for')
      }
    })()
  }

  // What happens on each edge of "your agent is blocked in the terminal".
  //
  // An EFFECT EVENT, so it reads the current presentation and visibility without
  // making them triggers: this must fire when the AGENT's state changes and at no
  // other time. A pane that re-ran this because it gained focus would take a user
  // to the terminal for a dialog that had been up, unchanged, for a minute.
  const onWaitEdge = useEffectEvent((nowWaiting: boolean) => {
    if (nowWaiting) {
      // Take them there — but only if they are watching this chat. A surprise
      // navigation in a pane nobody is looking at is worse than the banner they
      // will find when they come back, and the banner is up either way.
      //
      // AN ESCORT ONLY EXISTS BECAUSE THE TERMINAL IS SOMEWHERE ELSE. In split it
      // is not: it is on screen, beside the chat, and there is nothing to move
      // anybody to. So the split declines the whole transaction — no navigation
      // now, and (because escortRef stays 'none') no return trip and no offer
      // when the prompt clears. Collapsing the split to terminal-only would be
      // strictly worse than doing nothing: it would take away the chat half the
      // user is deliberately watching, to show them something already in view.
      if (presentation !== 'chat' || !userIsWatching()) return
      // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: this is a NAVIGATION on an edge, not a derivation. The surface must outlive the value that moved it: when `waiting` clears we may deliberately NOT switch back, so `presentation` cannot be computed from it.
      enterTerminal()
      escortRef.current = 'sent'
      return
    }
    // Cleared: somebody answered the dialog, or the CLI behind it is gone.
    const escort = escortRef.current
    escortRef.current = 'none'
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: see above; the offer belongs to one edge and is dismissible, so it cannot be derived either.
    setReturnOffered(false)
    if (escort === 'none') return
    if (escort === 'sent' && presentation === 'terminal' && userIsWatching()) {
      // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: see above.
      setPresentation('chat')
      return
    }
    // We cannot put them back — they navigated away, or they picked this surface
    // themselves — so we do not try. But we do not leave them here without a word
    // either: they are in a terminal Crowbar sent them to, for a reason that has
    // since gone away, and nothing else on screen would ever say so.
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: see above.
    if (presentation === 'terminal') setReturnOffered(true)
  })

  useEffect(() => {
    onWaitEdge(waiting)
  }, [waiting])

  // The user picking a surface ends Crowbar's claim on it. Picking Chat ends it
  // outright — they are already back, so there will be nothing to offer later.
  //
  // SPLIT COUNTS AS BEING BACK, for exactly that reason: it SHOWS the chat. An
  // offer to "return to chat" raised over a surface with the chat already on it
  // would be nonsense, so the claim is dropped rather than released.
  // A hotswap provider's terminal is already live — always has been, from spawn —
  // so picking it is a pure rendering choice, same as it always was. A provider
  // that hands its turn over instead (codex: attach declared, hotswap false) has
  // NO terminal session to render until Crowbar asks for one: switching to it
  // forks the native view first (switchToTerminal), and switching away from it
  // tears that view down and re-establishes the api connection (switchToNative).
  // Both are idle-only by construction — the control is disabled mid-turn (see
  // ViewSwitcher's handoverBlocked) — so neither races a live turn.
  const chooseSurface = (next: ChatPresentation) => {
    if (escortRef.current !== 'none') escortRef.current = next === 'terminal' ? 'released' : 'none'
    if (next !== 'terminal') setReturnOffered(false)
    // Arrive with the caret where the user just was. Coming from Terminal they
    // were typing at the CLI; coming from Chat they were typing a prompt.
    if (next === 'split') setSplitFocus(presentation === 'terminal' ? 'terminal' : 'chat')

    // An unresolved provider (catalogue not loaded yet, or this chat's provider
    // simply isn't in it) defaults to hotswap — the ORIGINAL, always-synchronous
    // behaviour every existing surface relies on — never to "must call the new
    // endpoint": that direction fails outright the instant it does (no
    // workspace/project scope to route through), where defaulting the other way
    // just costs a non-hotswap provider one extra render before its capability
    // loads in, same as any other capability-gated control.
    const chatProvider = providers.find((p) => p.id === activeProviderId)
    const hotswap = chatProvider ? chatProvider.hotswap === true : true
    if (hotswap || next === presentation) {
      setPresentation(next)
      return
    }
    if (next === 'terminal') {
      enterTerminal()
      return
    }
    if (presentation === 'terminal') {
      void (async () => {
        try {
          await switchToNative(wsId, shownChatId)
        } finally {
          setPresentation(next)
        }
      })()
      return
    }
    setPresentation(next)
  }

  // ⌘/ used to cycle the provider (see the effect above); it now toggles this
  // chat between its Chat and Terminal surfaces instead, the same pair
  // ViewSwitcher's own tabs flip between — this is just the keyboard route onto
  // the same chooseSurface call.
  //
  // Same guards as the cycle-provider effect above, for the same reasons: a
  // focused xterm stopPropagations a bubble-phase listener, so this must be a
  // CAPTURE-phase window listener; and a retained (hidden) workspace stays
  // mounted under display:none, so the wsId check keeps a background chat from
  // swallowing the key and flipping a surface nobody can see.
  const toggleViewChord = useEffectiveChordMap()[AGENT_TOGGLE_VIEW_MODE]
  const onToggleViewMode = useEffectEvent(() => {
    chooseSurface(presentation === 'terminal' ? 'chat' : 'terminal')
  })

  useEffect(() => {
    if (!isActivePane || !isVisible || !toggleViewChord) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (!eventMatchesChord(e, toggleViewChord)) return
      if (getActiveWorkspaceId() !== wsId) return
      e.preventDefault()
      e.stopPropagation()
      onToggleViewMode()
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [isActivePane, isVisible, toggleViewChord, wsId])

  // The banner's own button. They are going because Crowbar asked them to, so it
  // owes them the way back — but it did not move them, so it must not move them
  // back unasked either. 'released' is exactly that: offer, never take.
  //
  // In split there is nowhere to go, so the button does the only useful thing
  // left: it puts the CARET in the terminal, which is what the user wanted from
  // it anyway. It must not collapse the split — see onWaitEdge.
  const openTerminalFromBanner = () => {
    if (splitting) {
      setSplitFocus('terminal')
      terminalApiRef.current?.focus()
      return
    }
    escortRef.current = 'released'
    enterTerminal()
  }

  // The one reason this blank chat cannot be typed into right now, if there is
  // one — rendered INSIDE AgentEmptyDocument's own control-bar slot (the
  // model/effort/attach/send row), in place of it, rather than as a separate
  // row stacked above the document the way this used to sit. That old
  // placement free-floated above the composer's own control bar instead of
  // riding it, and a resize could visibly separate the two; this one shares
  // AgentEmptyDocument's single `place()` transform, so there is nothing left
  // to separate. Same mutually exclusive states as
  // waitingBannerShown/revivingBannerShown above, same precedence order. A
  // dormant blank chat needs none: typing into it starts the agent.
  const blankSignpost = waitingBannerShown ? (
    <AgentTerminalWaitBanner
      kind={waitKind ?? ''}
      providerLabel={providers.find((p) => p.id === activeProviderId)?.displayName ?? ''}
      onOpenTerminal={openTerminalFromBanner}
    />
  ) : revivingBannerShown && revival ? (
    <div data-testid="agent-reviving-banner">
      <ComposerSignpost reason="reviving" message={revival.message} onOpenTerminal={() => {}} />
    </div>
  ) : undefined

  // Clicking the gutters or the column's padding focuses the terminal.
  //
  // Those regions LOOK like part of the chat — they are the same bg-background, and the
  // whole point of the centred column is that its surroundings read as breathing room
  // rather than as somewhere else. But they are plain divs, so by default a mousedown
  // there does the opposite of what it looks like: it BLURS the terminal's textarea and
  // the user's next keystroke goes nowhere. preventDefault is what stops that blur; the
  // focus() then puts the caret back where the user obviously meant to click.
  //
  // The guard is a WHITELIST — the target must BE the root or the column, not merely be
  // inside them. A blacklist ("not the terminal, not a button") would silently start
  // hijacking clicks the day anything else is added to this pane: the switcher, its
  // menu, a future toolbar. Landing directly on a layout div is exactly what "the user
  // clicked empty space" means, and nothing else can accidentally match it.
  const focusTerminalFromEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached' || presentation !== 'terminal') return
    if (e.target !== rootRef.current && e.target !== columnRef.current) return
    e.preventDefault()
    terminalApiRef.current?.focus()
  }

  // The split's version, and the reason it is a SEPARATE handler rather than a
  // looser guard on the one above: in split the pane's gutters and the column's
  // padding sit outside BOTH halves, so a click there names no surface at all and
  // must not be answered by handing the keyboard to the terminal — that is
  // precisely how a composer loses the keystroke the user is mid-way through.
  // Only the terminal half's own empty strip counts, and the whitelist says so.
  const focusTerminalFromSplitEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached') return
    if (e.target !== terminalSurfaceRef.current) return
    e.preventDefault()
    setSplitFocus('terminal')
    terminalApiRef.current?.focus()
  }

  return (
    // One flat surface — no card, no border, no raised panel, and NO background of its
    // own. The pane's content region already paints `bg-pane-background`, which is the
    // surface every pane in Crowbar sits on; an editor pane shows it by staying
    // transparent, and so do we. Painting an opaque bg-background here did not bleed
    // onto anything — it COVERED that shared surface, which is exactly why the chat
    // read as a different material from the Monaco tab next to it.
    //
    // We built this on CossUI's Frame first, faithfully, and seeing it live is what
    // settled it: a Frame's whole job is to lift a panel OFF its background, and a
    // chat pane does not want to be lifted off anything. The bordered card framed
    // the agent's empty middle instead of hiding it, its squared top had nothing to
    // meet once the column was centred, and the switcher — outside the card — read
    // as a stray button on the desktop. Frame itself is untouched and still there
    // for surfaces that DO want to be raised.
    //
    // The gutters and the column's padding are DEAD SPACE that looks like part of the
    // chat, so clicking them focuses the terminal instead of blurring it — see
    // focusTerminalFromEmptySpace.
    <div
      ref={rootRef}
      // Dead-space layout chrome, not a control: the whitelist above already
      // guarantees this handler only fires when the click landed on the empty
      // div itself, never on real content. role="presentation" tells AT the
      // same thing — this wrapper carries no semantics of its own, and every
      // actually-interactive descendant (the terminal, the switcher) keeps
      // its own role untouched.
      role="presentation"
      onMouseDown={focusTerminalFromEmptySpace}
      className="flex h-full w-full flex-col"
    >
      {/* THE COLUMN. The terminal and the switcher live inside it, and the padding
          is on the column rather than on either of them — the switcher cannot
          drift out of line with the agent's first character, because they are
          inset by the SAME box. Alignment is structural here, not a hand-tuned
          pixel — which is exactly what it was before, and it broke every time
          anything moved.

          The cap (and its padding) is what makes gutters appear only when
          there is room to spare: wide pane → real gutters; narrow pane → the
          column just fills it. It resizes the PTY, so the agent genuinely
          re-wraps to ~106 columns instead of running lines to 164 — which is a
          TERMINAL concern, not a chat one: the chat's own reading column
          (`.center`, 768px) is already narrower than this cap and centers
          itself regardless of how wide its scroller is, and `.scroll`'s own
          padding-top (transcript.css) already gives the first message its
          breathing room. Capping the column here too, and padding it, used to
          nest the chat's scroll box a SECOND time inside both, which pushed
          its native scrollbar and its top edge in from the pane's real corner
          to this box's edge, then in again by the padding — so pure chat drops
          the cap, the top gutter and the RIGHT gutter (the scrollbar's own
          edge), same as split already does. The LEFT gutter stays: it is the
          one edge with something else beyond it — the app's own sidebar — and
          the composer's glass (`.dissolve`, composer.css) reaching flush to a
          real neighbour read as smudging it, where reaching flush to the
          pane's own scrollbar or top edge reads as intended. Only a lone
          terminal keeps the full wrapped width and all four gutters. */}
      <div
        ref={columnRef}
        className={cn(
          // The scope shared controls are styled under. The chat's own stylesheet
          // is scoped to `.agent-chat`, which the TERMINAL surface is not inside —
          // so the surface switcher sitting in its strip came out with no styling
          // at all. Anything both surfaces draw hangs off this instead.
          'agent-chat-pane',
          'mx-auto flex min-h-0 w-full flex-1 flex-col',
          splitting || presentation === 'chat' ? 'max-w-none' : 'max-w-4xl',
          // Pure chat keeps only its LEFT gutter — see above — dropping the
          // top and right ones this presentation would otherwise share with
          // terminal/split.
          splitting || presentation !== 'chat' ? 'px-4 pt-4' : 'pl-4',
        )}
      >
        <div
          ref={splitContainerRef}
          className={cn(
            'relative min-h-0 flex-1',
            splitting && (splitStacked ? 'flex flex-col' : 'flex flex-row'),
          )}
        >
          {/* Both presentations stay mounted. Keeping the queue mounted is what makes
              a Terminal detour non-destructive; keeping xterm mounted preserves its
              screen model while Chat is in front. Only the selected surface is active.

              `hidden` IS THE DORMANCY MECHANISM, not a cosmetic. A surface under
              display:none stops laying out, stops observing and — for xterm —
              stops drawing, which is why chat and terminal cost nothing while the
              other is up. Split deliberately gives that up for BOTH halves: it is
              a diagnostic, it is off by default, and the whole point is that the
              two are rendering at the same instant so a discrepancy between them
              is a discrepancy in the data rather than in the timing. */}
          <div
            ref={chatSurfaceRef}
            data-testid="agent-chat-surface"
            data-surface-focused={splitting ? String(splitFocus === 'chat') : undefined}
            onFocusCapture={splitting ? () => setSplitFocus('chat') : undefined}
            className={cn(
              splitting
                ? // NO CARD. Split is two surfaces with a handle between them —
                  // a border and a radius around each half turns a diagnostic
                  // into two floating panels, and the focus ring drew a box
                  // around whichever one you were typing in. The caret already
                  // says that.
                  'relative min-h-0 min-w-0 shrink grow-0'
                : // `hidden` IS THE DORMANCY MECHANISM — see the doc comment
                  // right above this whole div for why that's load-bearing,
                  // not cosmetic.
                  cn('h-full', presentation !== 'chat' && 'hidden'),
            )}
            style={{
              zoom: chatZoom,
              ...(splitting ? { flexBasis: `${splitSizes[0]}%` } : undefined),
            }}
          >
            {/* The trust/reviving/idle signpost, when this blank chat has one,
                rides inside AgentEmptyDocument's own control-bar slot —
                `blankSignpost` below — rather than rendering here as a
                sibling. With messages instead of AgentEmptyDocument, the
                composer becomes the signpost itself (AgentChatView's
                `terminalWait`), so passing this down too would put the same
                question on screen twice; `blankSignpost` only ever reaches
                the blank branch inside AgentChatView. */}
            <AgentChatView
              key={`${wsId}:${shownChatId}`}
              ref={chatViewRef}
              wsId={wsId}
              chatId={shownChatId}
              providerId={activeProviderId}
              providers={providers}
              switchDisabled={
                switchInFlight ||
                promptReplacing ||
                deliveryPending ||
                attachment.state === 'reviving'
              }
              working={working}
              compacting={compacting}
              turnRevision={turnRevision}
              live={attachment.state === 'attached' || promptReplacing}
              canSend={canSend}
              sessionNote={sessionNote}
              revival={revival}
              // In split the chat is genuinely in front of the user, so it is
              // genuinely active: it dispatches its queue, refreshes its catalog
              // and answers the barrier exactly as it does on its own.
              active={presentation === 'chat' || splitting}
              // Unknown chats never resolve, so this doubles as "give up polling a
              // chat that will 404 forever" — not just tab visibility.
              visible={isVisible && known}
              isActivePane={isActivePane}
              // The daemon has confirmed shownChatId does not exist (a stale
              // pane from a wiped/reseeded backend, or a chat deleted from
              // under an open tab). Nothing in this pane can ever resolve, so
              // close it rather than leave a permanent "Couldn't load" error
              // with no way back.
              //
              // Gated on `known`: `wsId` here is this AgentChatPane's own
              // AMBIENT workspace (from PaneContainer's WorkspaceStoreContext),
              // not necessarily the chat's real owning workspace. WorkspaceHost
              // keeps several WorkspaceViews mounted at once (keep-alive), each
              // rendering its OWN copy of the shared pane tree — `pane.chatId`
              // is a single window-level field (Task 26), so opening a chat
              // makes EVERY mounted WorkspaceView's AgentChatPane try to show
              // it, including ones whose ambient wsId is a different workspace
              // entirely. That copy's ledger fetch 404s against the wrong scope
              // — a routing mismatch, not evidence of deletion — and closePane
              // mutates the shared store, so an ungated close here would wipe
              // the chat out of the ONE copy that was rendering it correctly.
              // `known` (agentChats.chats for THIS ambient workspace) is false
              // both while its own chat list is still loading and, permanently,
              // when the chat simply isn't this workspace's to show — neither
              // is a deletion signal. Once this workspace's own list confirms
              // the chat (`known` flips true) a later 404 is trustworthy again;
              // a chat that never becomes known here is instead cleaned up by
              // the vanished-chat sweep in use-workspace-agent-chats-stream.ts,
              // which diffs THIS workspace's own list, not a foreign fetch.
              onChatGone={() => {
                if (!known) return
                windowPaneStore.getState().paneActions.closePane(paneId)
              }}
              onOpenTerminal={() => {
                // Unchanged outside split — the chat view's own way through to the
                // terminal, with no claim on bringing anybody back. In split there
                // is nothing to open, so it hands over the CARET instead; see
                // openTerminalFromBanner for why collapsing would be worse.
                if (splitting) {
                  setSplitFocus('terminal')
                  terminalApiRef.current?.focus()
                  return
                }
                enterTerminal()
              }}
              terminalWaiting={waiting}
              terminalWaitKind={waitKind ?? ''}
              headerClearancePx={headerClearancePx}
              transcriptHeaderClearancePx={transcriptHeaderClearancePx}
              blankSignpost={blankSignpost}
              presentation={presentation}
              splitEnabled={splitEnabled}
              onSelectPresentation={chooseSurface}
              settledPrompts={settledPrompts}
              abandonedPrompts={abandonedPrompts}
              streamingMessages={streamingMessages}
              reasoning={reasoning}
              toolOutput={toolOutput}
              plan={plan}
              onStreamingSettled={handleStreamingSettled}
              // A send may replace the CLI (restart_tui) or revive a dormant
              // chat; either way it is this pane's own request in flight, so the
              // composer stays an input rather than a "starting" signpost.
              onPromptDispatchStart={() => setPromptReplacing(true)}
              // Read the chat back (a versioned apply, never a write of our own)
              // before letting go, so the runner the send placed is in the store
              // before `live` is next judged.
              onPromptDispatchSettled={() => {
                void refreshChatWorking()
                  .catch(() => false)
                  .finally(() => setPromptReplacing(false))
              }}
              onTerminalSurface={onTerminalSurface}
              onRefreshChat={refreshChatWorking}
              provider={effectiveProviderId}
              model={effectiveModel}
              effort={effectiveEffort}
              launchModel={launchModel}
              // `known` is this pane's whole basis for "do we know what this
              // chat is" — until it flips, effectiveModel/Effort are just
              // their '' fallbacks, which the send would otherwise commit as
              // a deliberate clear. See AgentChatViewProps.selectionKnown.
              selectionKnown={known}
              onSelectionChange={stageSelection}
              onSelectionCommitted={commitSelection}
              onQueueCountChange={setQueuedPromptCount}
              onCancelableQueueCountChange={setCancelablePromptCount}
              onDeliveryPendingChange={setDeliveryPending}
              onBlankChange={setChatBlank}
            />
          </div>

          {splitting && (
            <PaneSash
              direction={splitStacked ? 'vertical' : 'horizontal'}
              sizes={splitSizes}
              containerRef={splitContainerRef}
              firstPaneRef={chatSurfaceRef}
              secondPaneRef={terminalSurfaceRef}
              onResizeCommit={setSplitSizes}
              // The TUI's floor, not the pane grid's — see SPLIT_MIN_HALF_PX.
              minPx={splitStacked ? SPLIT_MIN_STACKED_PX : SPLIT_MIN_HALF_PX}
            />
          )}

          <AgentTerminalSurface
            ref={terminalSurfaceRef}
            wsId={wsId}
            chatId={shownChatId}
            attachment={attachment}
            presentation={presentation}
            splitting={splitting}
            focused={splitFocus === 'terminal'}
            basis={splitSizes[1]}
            isActivePane={isActivePane}
            isVisible={isVisible}
            title={title}
            gridSlack={gridSlack}
            providers={providers}
            activeProviderId={activeProviderId}
            switchDisabled={
              switchInFlight ||
              promptReplacing ||
              deliveryPending ||
              attachment.state === 'reviving'
            }
            splitEnabled={splitEnabled}
            working={working}
            onSwitchProvider={handleSwitch}
            onSelectPresentation={chooseSurface}
            onTakeFocus={() => setSplitFocus('terminal')}
            onDeadSpaceMouseDown={focusTerminalFromSplitEmptySpace}
            terminalRef={terminalApiRef}

            onStartSession={startSession}
          />
        </div>

        {presentation === 'terminal' && returnOffered && (
          <AgentReturnToChatNotice
            onReturn={() => chooseSurface('chat')}
            onDismiss={() => setReturnOffered(false)}
          />
        )}

        {presentation === 'terminal' && queuedPromptCount > 0 && (
          <div className="flex items-center justify-between gap-3 border-t py-2 text-muted-foreground text-xs">
            <span>
              {queuedPromptCount} {queuedPromptCount === 1 ? 'prompt' : 'prompts'} pending in Chat
            </span>
            <div className="flex items-center gap-1">
              <Button size="xs" variant="ghost" onClick={() => setPresentation('chat')}>
                Return to Chat
              </Button>
              {cancelablePromptCount > 0 && (
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label="Cancel unsent prompts"
                  tooltip="Cancel unsent prompts"
                  onClick={() => chatViewRef.current?.cancelUnsentPrompts()}
                >
                  <TrashIcon />
                </Button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

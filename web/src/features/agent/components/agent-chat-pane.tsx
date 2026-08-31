import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  getChat,
  resumeChat,
  switchProvider,
  switchToNative,
  switchToTerminal,
} from '@/features/agent/api/agent-api'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { AGENT_CYCLE_PROVIDER } from '@/features/keymaps/registry'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { acceptChatRead, claimChatRead } from '@/features/agent/lib/chat-read-order'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import {
  SPLIT_MIN_HALF_PX,
  SPLIT_MIN_STACKED_PX,
  useChatPresentation,
  useTerminalGridSlack,
} from '@/features/agent/hooks/use-chat-presentation'
import { PaneSash } from '@/features/panes/components/pane-sash'
import { cn } from '@/lib/utils'
import {
  AgentReturnToChatNotice,
  AgentTerminalWaitBanner,
} from '@/features/agent/components/agent-terminal-wait-banner'
import { AgentChatView, type AgentChatViewHandle } from '@/features/agent/chat/agent-chat-view'
import type { ComposerRevival } from '@/features/agent/composer/lib/composer-state'
import {
  AgentTerminalSurface,
  type TerminalAttachment,
} from '@/features/agent/terminal/agent-terminal-surface'

// seedAttach pre-seeds the terminal-store mapping (connectionId = terminalSessionId)
// plus the localStorage reconnect backstop, so XtermTerminal's
// resolveTerminalConnection ATTACHES the agent's already-running PTY instead of
// spawning a fresh shell.
//
// Why this attaches (and does not spawn): XtermTerminal mounts with
// `sessionId = terminalSessionId`, reads getSession(sessionId).connectionId — now
// pre-seeded to the same id — and passes it to resolveTerminalConnection as
// `storeConnectionId`. On a fresh mount there is no live WS transport yet, so the
// resolver lists the daemon's live sessions, finds this PTY among them, and calls
// terminalAttach (the in-memory-store reuse branch).
//
// It must therefore NEVER be handed a dead PTY: resolveTerminalConnection's fallback
// for an unknown connection id is createTerminal(), so seeding a dead id would spawn a
// BARE SHELL inside the agent pane and persist that shell into the reconnect map.
//
// The caller's guard used to be TWO questions — is the segment still `active`, AND does
// the daemon still list its PTY as live — because those two could disagree. They cannot
// any more. A chat's terminalSessionId comes from its LIVE RUNNER, and a live-runner row
// exists exactly while its PTY does, so `liveRunnerId` being present IS the liveness
// answer. One authority, no second round trip, nothing left to drift.
function seedAttach(wsId: string, terminalSessionId: string): void {
  useTerminalStore.getState().updateSession(terminalSessionId, { connectionId: terminalSessionId })
  saveReconnect(wsId, terminalSessionId, terminalSessionId)
}

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

// The pane's attach outcome.
//
// `pending` is the pre-resolution state and renders nothing. It is NOT `idle`: it means
// the chat list has not reached the store yet, so we do not know whether this chat is
// live. Auto-reviving here — or flashing the Resume button — would offer to spawn a
// second CLI onto a chat that may well already have one.
//
// `reviving` is a spawn WE ASKED FOR that has not landed yet: a revive (automatic or
// from the button) or a provider switch. It carries its own message because those are
// different sentences to the user.
//
// `idle` is a chat with NO RUNNER on it — dormant — that we are NOT currently bringing
// back. Its reason decides what we say, and each is reachable one way only:
//
//   'failed' — a revive was tried and could not start a CLI (not installed, spawn
//              failed, or the chat has no conversation for the backend to resume into).
//              Retryable by the button, and the footer's dropdown can continue the
//              conversation with a different provider instead. THIS IS THE ONLY STATE
//              THE USER SHOULD EVER REACH BY OPENING A CHAT — opening a dormant one
//              revives it, so the button appears only where an attempt actually failed.
//   'exited' — the CLI died and we will NOT bring it back unasked: either its PTY died
//              under the open pane (see handleSessionGone), or this chat has already
//              spent its one revive on this mount (see attemptedRef). Honest, and it
//              cannot be reached by merely opening a dormant chat.
// Declared beside the surface that renders it — the terminal is the only thing
// that cares what a runner attachment looks like.
type Attachment = TerminalAttachment

interface AgentChatPaneProps {
  /** The chat this pane is pointed at. NOT stable for its life: the pane re-points it. */
  chatId: string
  /** The runner this pane follows, or '' when it has none (a dormant chat). */
  runnerId: string
  wsId: string
  /** The `PaneGroup` this surface renders inside — the thing that gets
   *  re-pointed (`paneActions.setPaneChat`) when the runner moves. Was a
   *  buffer id until Task 1 made `chatId`/`runnerId` fields on the pane
   *  itself; a chat has not been a buffer since. */
  paneId: string
  isActivePane: boolean
  /**
   * Whether this tab is the ACTIVE (visible) tab in its pane — distinct from
   * isActivePane (whether the pane has focus). The pane keeps every chat mounted
   * `visibility:hidden` for keep-alive, so a chat can be MOUNTED-BUT-HIDDEN. This
   * gates auto-revive: a hidden DORMANT chat must never spawn a CLI (N hidden
   * dormant tabs would each spawn one on workspace load) — it revives only once it
   * becomes visible. An already-attached chat stays attached while hidden.
   */
  isVisible: boolean
}

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
}: AgentChatPaneProps) {
  const store = useWorkspaceStore()

  // Where is MY runner? '' when it is nowhere — it exited, or a switch replaced it. A
  // chat is live exactly while a runner is placed on it, so this lookup is also what
  // makes a MOVE visible: the runner turns up under a different chat id.
  const runnerChatId = useStore(store, (s) =>
    runnerId ? (s.agentChats.chats.find((c) => c.liveRunnerId === runnerId)?.id ?? '') : '',
  )

  // The chat this pane is SHOWING: my runner's chat if it still has one (it moved, and
  // the tab follows), else the chat the tab was pointed at (which may be dormant).
  const shownChatId = runnerChatId || chatId

  // Does the store KNOW this chat at all? "Not in the store yet" (the seed is in flight)
  // is not "dormant", and must not render the Resume button — see `pending` above.
  const known = useStore(store, (s) => s.agentChats.chats.some((c) => c.id === shownChatId))
  // The runner on the shown chat — mine, or whoever replaced it. '' = dormant.
  const liveRunnerId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.liveRunnerId ?? '',
  )
  // That runner's PTY, if it has one to show right now. NOT a second liveness
  // signal — a non-hotswap api-transport runner (codex) is legitimately live
  // with this empty (nothing attached), so liveRunnerId above is the only
  // thing that means "no runner, nothing to attach".
  const sessionId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.terminalSessionId ?? '',
  )
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.activeProviderId ?? '',
  )
  const working = useStore(store, (s) => s.agentChats.working[shownChatId] ?? false)
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
  const providers = useStore(store, (s) => s.agentChats.providers)
  // The provider this chat last ran under — what a failed revive has to NAME ("Claude
  // isn’t installed"), since a dormant chat has no live runner to ask.
  const chatProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.activeProviderId ?? '',
  )

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

  // The message the agent is mid-way through saying. Selected as two PRIMITIVES
  // rather than as the object: the object is replaced on every frame — roughly
  // 1.4 a second — so a selector returning it would re-run every consumer on
  // identity alone even when the text had not moved.
  const streamingId = useStore(store, (s) => s.agentChats.streamingMessages[shownChatId]?.id)
  const streamingText = useStore(store, (s) => s.agentChats.streamingMessages[shownChatId]?.text)

  const [attachedState, setAttachment] = useState<Attachment>({ state: 'pending' })
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
  // `pending` while the chat list is still in flight is DERIVED, not written by
  // the attach effect below. Dormancy is unknowable until the list lands, so
  // there is nothing for the machine to record — the pane simply has nothing to
  // show yet. Keeping it out of the state means the effect's only job is
  // spawning and attaching CLIs, and a `known` flip no longer costs an extra
  // render to undo a value the effect had just written.
  const attachment: Attachment = known ? attachedState : { state: 'pending' }

  // The composer's own words for `attachment`, refined past the plain `live`
  // boolean it gets alongside this — but only on the chat side of the gate.
  // Both surfaces stay mounted (see the dormancy note by the split container
  // below), and the terminal surface carries its OWN copy of this same
  // reviving/idle treatment, gated the mirror-image way. Without this check
  // BOTH copies would sit in the document at once — a visible duplicate in
  // split, and one hidden behind `display:none` and just as duplicate to
  // anything that reads the DOM instead of looking at it.
  const revival: ComposerRevival | undefined =
    presentation === 'terminal'
      ? undefined
      : attachment.state === 'reviving'
        ? { state: 'reviving', message: attachment.message }
        : attachment.state === 'idle'
          ? { state: 'idle', reason: attachment.reason }
          : undefined

  // The session this pane currently WANTS attached, readable from a callback that
  // must not re-identify on every attach. handleSessionGone compares against it.
  // Tracked as the id STRING, not the Attachment object: `attachment` is rebuilt
  // every render while `known` is false, so depending on the object would re-run
  // this on every render for no change.
  const attachedSessionId = attachment.state === 'attached' ? (attachment.sessionId ?? '') : ''
  const desiredSessionRef = useRef('')
  useEffect(() => {
    desiredSessionRef.current = attachedSessionId
  }, [attachedSessionId])

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

  // Re-point the PANE at what this surface is actually showing. This is the write that
  // makes the view follow: pane-container feeds the pane's chatId/runnerId straight
  // back in as our props, so the next render is already looking at the new chat.
  //
  // It converges in one step and cannot loop — once the pane holds the pair computed
  // here, the effect's deps are unchanged and it does not re-run, and `setPaneChat`'s
  // own writes are same-value no-ops under immer. Writing null as the runnerId of a
  // dormant chat is deliberate: a pane must not go on claiming a runner that no longer
  // exists.
  //
  // `setPaneChat` (not the deleted `repointAgentChatBuffer`, which guarded on a buffer
  // type Task 1 removed and had therefore been a permanent no-op) also archives the
  // conversation the CLI walked OUT of into Recents when this is a real move — spec
  // §5.5's "the view dies, the row does not."
  useEffect(() => {
    if (!known) return // nothing authoritative to re-point at yet
    if (shownChatId === chatId && liveRunnerId === runnerId) return
    windowPaneStore.getState().paneActions.setPaneChat(paneId, shownChatId, liveRunnerId || null)
  }, [store, paneId, known, chatId, runnerId, shownChatId, liveRunnerId])

  // NO TAB RELABEL HERE ANY MORE. A chat used to be a buffer whose `name` was the tab
  // label, so a title arriving after the tab opened (the agent auto-titles the chat via
  // WS `title_set`, the user renames it from the sidebar, or the runner MOVED us to a
  // fresh chat with a title of its own) had to be mirrored onto that buffer. Task 17's
  // `ChatHead` reads `agentChats.chats.find(c => c.id === chatId)?.title` straight from
  // the store instead, so all three cases are live for free — and the third one now
  // works, because the effect above re-points the PANE's own chatId rather than a
  // buffer that has not existed since Task 1.

  // THE REVIVE BUDGET: ONE PER CHAT, PER PANE MOUNT. Every id in here has already had a
  // revive fired at it by this pane, and will never get another one unasked — whatever
  // the outcome, and however many times the chat goes dormant again.
  //
  // This is the entire safety argument for reviving automatically, so it is spent BEFORE
  // the request goes out (see revive), not after it comes back. A budget spent on success
  // cannot loop; a budget spent on failure cannot retry-storm; and React's StrictMode
  // double-effect cannot double-spawn, because the second pass finds the id already here.
  const attemptedRef = useRef(new Set<string>())

  // A spawn THIS PANE asked for is in flight (a provider switch). Its chat goes
  // transiently dormant on the way — the backend displaces the outgoing CLI before the
  // incoming one exists, and the `displaced` frame refetches the chat into exactly that
  // gap. Nothing may read that gap as "this chat needs reviving": it needs the CLI that
  // is already coming.
  const switchingRef = useRef(false)

  // Is this pane still on screen? Both reads below can land after it is gone — a workspace
  // switch, a closed pane — and applying one then writes into a torn-down store AND spends
  // a chat-read-order slot, which makes a live seed elsewhere retry for a write nobody can
  // see. Set on every mount, not just once, so StrictMode's double-effect does not leave it
  // false for the run that survives. The hook's own reads have `cancelled` for this.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // adopt reads the chat back and settles the pane on what the SERVER says: attach to the
  // runner now on it, or report that there still is none (false). It only ever READS, so
  // it cannot spawn anything; the caller has already done the spawning.
  //
  // Reading is not belt-and-braces. The WS would push the same facts eventually, but this
  // way the pane settles on the ACT rather than whenever a frame happens to arrive — and,
  // crucially, it settles AT ALL: a CLI that died on startup leaves the chat dormant, and
  // this read says so, where waiting for a frame that is never coming would spin forever.
  //
  // IT IS ALSO ONE OF TWO RACERS. The resume this follows makes the daemon publish
  // `started`, and use-workspace-agent-chats-stream refetches the same chat off that
  // frame — usually ISSUING FIRST, since the socket push beats the POST response. The
  // daemon can answer that first read from before the placement it just announced, so it
  // can land LAST carrying "dormant" and overwrite the live row this one just wrote. The
  // pane's one revive is spent by then, so it settles on "This agent has exited" over a
  // CLI that is alive — the confirmed live bug. Both reads therefore go through the one
  // ordering registry, and the loser is discarded rather than applied.
  const adopt = useCallback(async (): Promise<boolean> => {
    const ticket = claimChatRead()
    const fetched = await getChat(wsId, shownChatId)
    if (!mountedRef.current) return false
    const s = store.getState()
    if (acceptChatRead(wsId, fetched.id, ticket)) {
      s.upsertAgentChat(fetched, ticket)
      s.setAgentChatWorking(fetched.id, fetched.working === true)
    }
    // Settle on the STORE, not on our own payload: when a later-issued read has already
    // applied, that row is the newer truth and this one is a snapshot of the past.
    // Attaching off the older answer would seed a PTY the server has already moved past.
    const chat = store.getState().agentChats.chats.find((c) => c.id === fetched.id) ?? fetched
    // liveRunnerId ALONE is liveness — terminalSessionId is not a second vote on it.
    // A non-hotswap api-transport runner (codex) is legitimately live with nothing
    // attached: empty here means "no terminal to show right now", not "no runner".
    if (!chat.liveRunnerId) return false
    windowPaneStore.getState().paneActions.setPaneChat(paneId, chat.id, chat.liveRunnerId)
    if (chat.terminalSessionId) seedAttach(wsId, chat.terminalSessionId)
    setAttachment({ state: 'attached', sessionId: chat.terminalSessionId || null })
    return true
  }, [store, wsId, paneId, shownChatId])

  // Re-check the aggregate after a prompt race. The prompt queue consumes only
  // this server-folded value; it never guesses busy state from a lifecycle kind.
  // Ordered against every other single-chat read for the same reason adopt is.
  const refreshChatWorking = useCallback(async (): Promise<boolean> => {
    const ticket = claimChatRead()
    const fetched = await getChat(wsId, shownChatId)
    const s = store.getState()
    // Torn down mid-flight: report the store's answer, write nothing, spend no slot.
    if (!mountedRef.current) return s.agentChats.working[fetched.id] === true
    if (!acceptChatRead(wsId, fetched.id, ticket)) {
      return s.agentChats.working[fetched.id] === true
    }
    s.upsertAgentChat(fetched, ticket)
    s.setAgentChatWorking(fetched.id, fetched.working === true)
    return fetched.working === true
  }, [store, wsId, shownChatId])

  // A selection the SERVER accepted. It is written here rather than inside the
  // picker because the store is the chat's owner, and the 202 carries no body and
  // no lifecycle frame — nothing else would bring the new pair back.
  const applySelection = useCallback(
    (model: string, effort: string) => {
      store.getState().setAgentChatSelection(shownChatId, model, effort)
    },
    [store, shownChatId],
  )

  const handlePromptSpawned = useCallback(async () => {
    await adopt()
  }, [adopt])

  // A spawn we asked for onto this chat did not put a CLI on it. Say so — and SPEND THE
  // CHAT'S BUDGET while we are at it, whichever path failed: the pane is now looking at a
  // dormant chat that has just demonstrably refused a CLI, and reviving it unasked would
  // only be the same failure down the other road. The user drives from here.
  const fail = useCallback(() => {
    attemptedRef.current.add(shownChatId)
    setAttachment({ state: 'idle', reason: 'failed' })
  }, [shownChatId])

  // revive brings the chat's last provider back into its own native session — the CLI
  // resumes exactly where the user left it. Fired automatically when the pane finds its
  // chat dormant (see the attach effect), and by the Resume button when that failed.
  //
  // Failure is a FIRST-CLASS OUTCOME, not an edge: the backend refuses outright to resume
  // a chat with no recorded conversation ("no conversation to resume" — a CLI that died
  // before its session-start hook ever fired leaves one), and the CLI itself may be gone
  // from the PATH. Both land in `idle: failed`, which is the one place the Resume button
  // still appears. It never retries by itself.
  const revive = useCallback(async () => {
    attemptedRef.current.add(shownChatId) // spend the budget BEFORE awaiting anything
    setAttachment({ state: 'reviving', message: 'Resuming this chat…' })
    try {
      await resumeChat(wsId, shownChatId)
      if (!(await adopt())) fail()
    } catch (err: unknown) {
      fail()
      const name = providers.find((p) => p.id === chatProviderId)?.displayName || 'the agent'
      toastSpawnFailure(err, name, 'resume')
    }
  }, [wsId, shownChatId, adopt, fail, providers, chatProviderId])

  // Attach to the runner's PTY, revive the chat if nobody is on it, or settle. The seeding
  // must happen BEFORE XtermTerminal mounts (React runs child effects first, so a terminal
  // rendered in the same commit would resolve its connection against an unseeded store) —
  // hence the state machine: this effect seeds, flips to `attached`, and the terminal
  // mounts on the next render.
  //
  // OPENING A DORMANT CHAT REVIVES IT. The agent PTY does not survive a daemon restart, so
  // "dormant" is the ordinary state of yesterday's conversation — and a user who clicks a
  // chat is asking for the chat, not for a button that asks whether they meant it. So the
  // pane brings the CLI back and shows the spinner; the exited copy and the button are
  // what a FAILURE looks like, not what an open looks like.
  //
  // This is not a re-run of the auto-revive that was deleted here. THAT one had to guess:
  // it could not tell "my CLI died" from "my CLI moved", so a moved runner read as a dead
  // one and it respawned onto the chat the user had just left — and only a retry counter
  // stopped it doing that forever. This pane cannot make that mistake, because it FOLLOWS
  // its runner (shownChatId) and so a move is never dormancy. What is left is a genuinely
  // dormant chat — and it is revived exactly ONCE, ever, per chat per mount:
  //
  //   !known                → pending. The chat list is still in flight and dormancy is
  //                           unknowable; reviving here could put a second CLI on a chat
  //                           that already has one. NEVER revive from here.
  //   switch in flight      → the incoming CLI is already on its way. Wait for it.
  //   dormant, budget left  → revive it, unattended. Spends the budget.
  //   dormant, budget spent → settle. The user drives from here (button, or a different
  //                           provider from the dropdown). No retry, no loop, ever.
  useEffect(() => {
    // Not knowable yet — `attachment` above already reads `pending` while this
    // is false, so there is nothing to write.
    if (!known) return
    // liveRunnerId, not sessionId — a non-hotswap api-transport runner (codex) is
    // legitimately live with an empty terminalSessionId (nothing attached right
    // now), and that must not read as dormant: it would fire a revive onto a chat
    // that already has a perfectly healthy runner on it.
    if (!liveRunnerId) {
      if (switchingRef.current) return // the switch's own spinner stands
      // ONLY THE VISIBLE TAB REVIVES. A dormant chat kept alive on a hidden tab must
      // NOT spawn a CLI: opening a workspace with N dormant chat tabs would otherwise
      // fire N revives at once, one CLI per hidden tab. A hidden dormant chat waits;
      // when the user switches to it (isVisible flips true, this effect re-runs) it
      // revives — exactly once, because the budget below is spent inside revive(). An
      // already-attached chat has a liveRunnerId and never reaches here, so it keeps
      // its live PTY while hidden.
      if (isVisible && !attemptedRef.current.has(shownChatId)) {
        void revive()
        return
      }
      // Budget spent, or hidden and waiting to become visible. Don't stomp a revive
      // still in flight, and don't overwrite a `failed` we have already earned with the
      // vaguer `exited`.
      setAttachment((a) =>
        a.state === 'reviving' || a.state === 'idle' ? a : { state: 'idle', reason: 'exited' },
      )
      return
    }
    if (sessionId) seedAttach(wsId, sessionId)
    setAttachment({ state: 'attached', sessionId: sessionId || null })
  }, [wsId, known, liveRunnerId, sessionId, shownChatId, revive, isVisible])

  // The attach above proves the PTY was alive when the store last spoke. The CLI can die
  // at any moment while the pane sits here — daemon restart, /exit, crash — and the
  // terminal's transport-drop reconnect would, by default, resolve the now-dead session
  // by spawning a fresh BARE SHELL into this frame. So the terminal runs attach-only and
  // reports the session gone instead, and we render the dormant state at once rather
  // than waiting for the backend to notice: the PTY's death and the daemon's knowledge
  // of it are not the same instant.
  //
  // A MOVE never lands here — a runner keeps its PTY when it changes conversation, so the
  // terminal sees nothing at all. That is exactly why this can be read as "the CLI died"
  // with no ambiguity, and why the pane never has to guess.
  //
  // IT DOES NOT REVIVE FROM HERE, and that is deliberate: this signal is the CLIENT
  // noticing, and the daemon has not necessarily noticed yet. A resume fired at this
  // instant can be answered with the STILL-RECORDED live runner ("already live, here it
  // is") whose PTY is the dead one we are holding — and seeding that id spawns the bare
  // shell seedAttach exists to prevent. So we say what we see, and let the AUTHORITY
  // speak: when the daemon reaps the runner the chat goes dormant in the store, the effect
  // above sees it, and — budget permitting — revives it there, off the server's verdict
  // instead of our own. A CLI that dies twice in one mount stays down.
  const handleSessionGone = useCallback((goneSessionId: string) => {
    if (switchingRef.current) return // our own switch killing the outgoing CLI: expected
    // A DISPLACED PTY REPORTS ITS DEATH LATE. Prompt submission replaces the CLI, so
    // the outgoing PTY dies by design — but the terminal notices the closed transport
    // whenever it notices, which can be well after adopt() has already attached the
    // replacement and the dispatch guard above has been dropped. Believing that report
    // latched `exited` over a chat whose runner the SERVER still lists as live, and the
    // React queue — which may only dispatch onto a live TUI — stalled there forever.
    //
    // So the guard is IDENTITY, not timing: only the session the pane still wants can
    // report that pane's agent gone. An id we no longer hold is the outgoing corpse.
    if (goneSessionId && desiredSessionRef.current && goneSessionId !== desiredSessionRef.current) {
      return
    }
    setAttachment({ state: 'idle', reason: 'exited' })
  }, [])

  // Switch the provider ON THE CHAT THE RUNNER IS IN NOW (shownChatId — not the chatId
  // the tab was opened on): after a /clear the pane is showing a different conversation,
  // and switching the one the user has left would spawn a CLI onto abandoned context
  // while the live one kept running unattended.
  //
  // It lands through the same adopt() as a revive, for the same two reasons: the pane must
  // settle on the act rather than on a frame, and a switch whose incoming CLI died on
  // startup must settle HONESTLY (dormant chat → `failed`) instead of spinning.
  //
  // A switch can fail for real, ordinary reasons — the target CLI is not installed, the
  // spawn fails — and without a catch the rejection is unhandled: the dropdown just closes
  // and nothing happens. Surface it, matching the write-path error handling in
  // agent-chats-panel (create/rename/delete).
  //
  // AWAITABLE, and its answer is load-bearing. Picking another provider's model
  // from the identity chip is one gesture but two writes, and the second is only
  // legal once the first has landed: the selection endpoint validates the model
  // against the chat's CURRENT provider, so writing `gpt-5.6-luna` while the chat
  // is still on claude is a 400 and the user watches their pick bounce.
  const handleSwitch = async (providerId: string): Promise<boolean> => {
    if (
      switchingRef.current ||
      promptReplacing ||
      deliveryPending ||
      attachment.state === 'reviving'
    )
      return false
    const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
    // Held across the whole request: the outgoing CLI is killed FIRST, so this window is
    // exactly the transient dormancy — and its dead PTY's onSessionGone — that nothing
    // must mistake for a chat needing revival.
    switchingRef.current = true
    setAttachment({ state: 'reviving', message: `Starting ${name}…` })
    try {
      await switchProvider(wsId, shownChatId, providerId)
      if (!(await adopt())) {
        fail()
        return false
      }
      return true
    } catch (err: unknown) {
      fail()
      // Status-aware: only a 424 actually means "that CLI is not installed". Blaming the
      // PATH for every failure sends the user hunting for a problem they do not have.
      toastSpawnFailure(err, name, 'switch to')
      return false
    } finally {
      switchingRef.current = false
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
      setPresentation('terminal')
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
    const chatProvider = providers.find((p) => p.id === chatProviderId)
    const hotswap = chatProvider ? chatProvider.hotswap === true : true
    if (hotswap || next === presentation) {
      setPresentation(next)
      return
    }
    if (next === 'terminal') {
      void (async () => {
        try {
          await switchToTerminal(wsId, shownChatId)
          await adopt()
          setPresentation('terminal')
        } catch {
          // Left where they were — a blocked/unavailable switch is reported by
          // whatever surfaces the request's own error today, not by moving them
          // to a view that never actually came up.
        }
      })()
      return
    }
    if (presentation === 'terminal') {
      void (async () => {
        try {
          await switchToNative(wsId, shownChatId)
        } finally {
          await adopt()
          setPresentation(next)
        }
      })()
      return
    }
    setPresentation(next)
  }

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
    setPresentation('terminal')
  }

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
      {/* THE COLUMN. Both the terminal and the switcher live inside it, and the
          padding is on the column rather than on either of them. That is the whole
          trick: the switcher cannot drift out of line with the agent's first
          character, because they are inset by the SAME box. Alignment is structural
          here, not a hand-tuned pixel — which is exactly what it was before, and it
          broke every time anything moved.

          The cap is what makes the padding appear only when there is room to spare:
          wide pane → real gutters; narrow pane → the column just fills it. And it
          resizes the PTY, so the agent genuinely re-wraps to ~106 columns instead of
          running lines to 164. Because the whole pane is one bg-background, the
          padding and the gutters are invisible — you see breathing room, not a box. */}
      <div
        ref={columnRef}
        className={cn(
          // The scope shared controls are styled under. The chat's own stylesheet
          // is scoped to `.agent-chat`, which the TERMINAL surface is not inside —
          // so the surface switcher sitting in its strip came out with no styling
          // at all. Anything both surfaces draw hangs off this instead.
          'agent-chat-pane',
          'mx-auto flex min-h-0 w-full flex-1 flex-col px-4 pt-4',
          // The reading column exists so one surface does not run to 164 characters.
          // Two surfaces have the opposite problem, so the split takes the pane.
          splitting ? 'max-w-none' : 'max-w-4xl',
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
                : cn('h-full', presentation === 'chat' ? '' : 'hidden'),
            )}
            style={splitting ? { flexBasis: `${splitSizes[0]}%` } : undefined}
          >
            <AgentChatView
              key={`${wsId}:${shownChatId}`}
              ref={chatViewRef}
              wsId={wsId}
              chatId={shownChatId}
              providerId={activeProviderId}
              providers={providers}
              onSwitchProvider={handleSwitch}
              switchDisabled={promptReplacing || deliveryPending || attachment.state === 'reviving'}
              working={working}
              turnRevision={turnRevision}
              live={attachment.state === 'attached' || promptReplacing}
              revival={revival}
              onRevive={() => void revive()}
              // In split the chat is genuinely in front of the user, so it is
              // genuinely active: it dispatches its queue, refreshes its catalog
              // and answers the barrier exactly as it does on its own.
              active={presentation === 'chat' || splitting}
              visible={isVisible}
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
                setPresentation('terminal')
              }}
              terminalWaiting={waiting}
              terminalWaitKind={waitKind ?? ''}
              presentation={presentation}
              splitEnabled={splitEnabled}
              onSelectPresentation={chooseSurface}
              settledPrompts={settledPrompts}
              streamingMessageId={streamingId}
              streamingMessageText={streamingText}
              onPromptDispatchStart={() => {
                switchingRef.current = true
                setPromptReplacing(true)
              }}
              onPromptDispatchSettled={() => {
                // The prompt endpoint may fail after terminating the outgoing
                // TUI. Reconcile before dropping the displacement guard, or the
                // pane can remain "attached" to a dead PTY forever (the session
                // change happened while the guard intentionally ignored it).
                void (async () => {
                  try {
                    if (!(await adopt())) fail()
                  } catch {
                    fail()
                  } finally {
                    switchingRef.current = false
                    setPromptReplacing(false)
                  }
                })()
              }}
              onPromptSpawned={handlePromptSpawned}
              onRefreshChat={refreshChatWorking}
              model={chatModel}
              effort={chatEffort}
              onSelectionChange={applySelection}
              onQueueCountChange={setQueuedPromptCount}
              onCancelableQueueCountChange={setCancelablePromptCount}
              onDeliveryPendingChange={setDeliveryPending}
              onBlankChange={setChatBlank}
            />

            {/* The composer's OWN reviving/idle signpost (`revival`, above) is
                the normal home for these now — but it lives inside the dock,
                and the dock does not exist on a blank chat (AgentEmptyDocument
                stands in for it there). A CLI dying before its first turn ever
                lands is not a rare edge of that: it is the ordinary shape of
                "opened a chat, picked a provider, the provider never came up."
                Proven wrong once already for terminal_wait below (dropping it
                outright broke `agent-chat-pane-terminal-wait.test.tsx`), and
                the same test-first check for reviving/idle
                (agent-chat-pane.test.tsx, "…for a chat with no messages yet")
                found the identical gap — so this is the fallback for exactly
                the window `chatBlank` covers, not a second copy of the
                composer's own signpost: `revival` above is already `undefined`
                whenever `chatBlank` would make this render, because
                `AgentChatView` never got far enough to read it. */}
            {presentation !== 'terminal' && chatBlank && attachment.state === 'reviving' && (
              <div className="absolute inset-x-4 top-2 flex items-center justify-center gap-2 rounded-lg border bg-popover/95 px-3 py-2 text-muted-foreground text-sm shadow-sm">
                <FlickerSpinner className="size-4 text-foreground" />
                {attachment.message}
              </div>
            )}
            {presentation !== 'terminal' && chatBlank && attachment.state === 'idle' && (
              <div className="absolute inset-x-4 top-2 flex items-center justify-between gap-3 rounded-lg border bg-popover/95 px-3 py-2 text-sm shadow-sm">
                <p className="min-w-0 text-muted-foreground">
                  {attachment.reason === 'failed'
                    ? 'Crowbar could not restart this agent. Check that its CLI is installed, then try again — or pick another provider below.'
                    : 'This agent has exited. Resume it to pick the conversation up where you left off.'}
                </p>
                <Button
                  className="shrink-0"
                  type="button"
                  variant="secondary"
                  size="sm"
                  data-testid="pane-resume"
                  onClick={() => void revive()}
                >
                  Resume
                </Button>
              </div>
            )}
            {waiting && (
              <div className="absolute inset-x-4 top-2 rounded-lg bg-popover shadow-sm">
                <AgentTerminalWaitBanner
                  kind={waitKind ?? ''}
                  providerLabel={
                    providers.find((p) => p.id === activeProviderId)?.displayName ?? ''
                  }
                  onOpenTerminal={openTerminalFromBanner}
                />
              </div>
            )}
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
            switchDisabled={promptReplacing || deliveryPending || attachment.state === 'reviving'}
            splitEnabled={splitEnabled}
            working={working}
            onSwitchProvider={handleSwitch}
            onSelectPresentation={chooseSurface}
            onTakeFocus={() => setSplitFocus('terminal')}
            onDeadSpaceMouseDown={focusTerminalFromSplitEmptySpace}
            onTerminalRef={(api) => {
              terminalApiRef.current = api
            }}
            onSessionGone={handleSessionGone}
            onRevive={() => void revive()}
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
                  <Trash2Icon />
                </Button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

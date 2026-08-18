import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { MessageSquareIcon, TerminalIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { getChat, resumeChat, switchProvider } from '@/features/agent/api/agent-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { AGENT_CYCLE_PROVIDER } from '@/features/keymaps/registry'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { getDefaultChatPresentation } from '@/features/settings/lib/chat-presentation'
import { cn } from '@/lib/utils'
import { AgentReturnToChatNotice, AgentTerminalWaitBanner } from './agent-terminal-wait-banner'
import { ProviderSwitchDropdown } from './provider-switch-dropdown'
import { AgentChatView, type AgentChatViewHandle } from './agent-chat-view'

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
type Attachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string }
  | { state: 'reviving'; message: string }
  | { state: 'idle'; reason: 'exited' | 'failed' }

interface AgentChatPaneProps {
  /** The chat this tab is pointed at. NOT stable for its life: the pane re-points it. */
  chatId: string
  /** The runner this tab follows, or '' when it has none (a dormant chat). */
  runnerId: string
  wsId: string
  /** The pane buffer backing this tab — the thing the pane re-points and relabels. */
  bufferId: string
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
  bufferId,
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
  // That runner's PTY. Empty exactly when liveRunnerId is: no runner, nothing to attach.
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

  const [attachedState, setAttachment] = useState<Attachment>({ state: 'pending' })
  // Seeded from the user's preference, never subscribed to it: a chat already open
  // keeps the surface it is on if the setting changes underneath it.
  const [presentation, setPresentation] = useState<'chat' | 'terminal'>(
    getDefaultChatPresentation,
  )
  // Whether the way back to the chat is being OFFERED. See AgentReturnToChatNotice:
  // it is what Crowbar shows when it moved somebody to the terminal and then
  // cannot move them back without overriding a choice they made themselves.
  const [returnOffered, setReturnOffered] = useState(false)
  // Re-seed PER CHAT, not per mount — that distinction is the whole fix. This pane
  // is RETAINED across chat selection, so a lazy useState initializer runs once for
  // the pane's entire life: turning the preference off and opening a chat did
  // nothing, and it only ever took effect after a full reload. That reads, fairly,
  // as "the setting is broken".
  //
  // Keyed on the CHAT rather than on the preference, so the two cases stay
  // distinct: a different chat lands on whatever the user prefers, while a chat
  // already on screen never jumps surface under someone because the preference
  // changed elsewhere. returnOffered goes with it — an offer to return belongs to
  // the chat that raised it, not to whichever one is shown next.
  const [seededFor, setSeededFor] = useState(shownChatId)
  if (seededFor !== shownChatId) {
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- accepted: React's documented "adjust state when a prop changes" pattern. An effect would paint the previous chat's surface for a frame first, which is the flicker this exists to avoid.
    setSeededFor(shownChatId)
    setPresentation(getDefaultChatPresentation())
    setReturnOffered(false)
  }
  const [queuedPromptCount, setQueuedPromptCount] = useState(0)
  const [cancelablePromptCount, setCancelablePromptCount] = useState(0)
  const [promptReplacing, setPromptReplacing] = useState(false)
  const [deliveryPending, setDeliveryPending] = useState(false)
  const chatViewRef = useRef<AgentChatViewHandle>(null)
  // `pending` while the chat list is still in flight is DERIVED, not written by
  // the attach effect below. Dormancy is unknowable until the list lands, so
  // there is nothing for the machine to record — the pane simply has nothing to
  // show yet. Keeping it out of the state means the effect's only job is
  // spawning and attaching CLIs, and a `known` flip no longer costs an extra
  // render to undo a value the effect had just written.
  const attachment: Attachment = known ? attachedState : { state: 'pending' }

  // The session this pane currently WANTS attached, readable from a callback that
  // must not re-identify on every attach. handleSessionGone compares against it.
  // Tracked as the id STRING, not the Attachment object: `attachment` is rebuilt
  // every render while `known` is false, so depending on the object would re-run
  // this on every render for no change.
  const attachedSessionId = attachment.state === 'attached' ? attachment.sessionId : ''
  const desiredSessionRef = useRef('')
  useEffect(() => {
    desiredSessionRef.current = attachedSessionId
  }, [attachedSessionId])

  // The two layout divs whose empty space belongs to the terminal, and the terminal's
  // own imperative handle — see focusTerminalFromEmptySpace.
  const rootRef = useRef<HTMLDivElement>(null)
  const columnRef = useRef<HTMLDivElement>(null)
  const terminalApiRef = useRef<{ focus: () => void } | null>(null)

  // A TERMINAL DOES NOT RENDER TO THE EDGE OF ITS OWN BOX.
  //
  // xterm draws a character grid: it fits `floor(width / cellWidth)` columns and also
  // holds back room for a scrollbar, so the canvas is NARROWER than the element that
  // contains it — here by 16px. That strip is never drawn to. Align the status line to
  // the terminal's ELEMENT and it lands perfectly on empty space, sitting visibly past
  // where the agent's text actually stops. (Which is exactly what happened: the box was
  // flush to the pixel while the switcher hung ~25px beyond the last column.)
  //
  // So measure the real thing — the canvas — and pad the status line by the difference.
  // It cannot be a constant: it moves with the font metrics, the pane width, and the
  // scrollbar reservation.
  const [gridSlack, setGridSlack] = useState(0)

  useEffect(() => {
    const column = columnRef.current
    if (attachment.state !== 'attached' || presentation !== 'terminal' || !column) {
      setGridSlack(0)
      return
    }

    const measure = () => {
      const element = column.querySelector('.xterm')
      const canvas = column.querySelector('.xterm canvas')
      // No canvas yet — the renderer has not drawn. The MutationObserver below will call
      // us back the moment it appears; do NOT fall back to 0, that is what pinned the
      // status line to the element's edge instead of the grid's.
      if (!element || !canvas) return
      const slack = element.getBoundingClientRect().right - canvas.getBoundingClientRect().right
      setGridSlack(Math.max(0, Math.round(slack)))
    }

    // Two different events change the answer, and BOTH are needed.
    //
    // MutationObserver — xterm builds its canvas asynchronously, after this effect's
    // first frame. Measuring only on mount reads a terminal that has not rendered yet
    // and silently yields 0, and since nothing then resizes, it stays 0 forever. That is
    // precisely the bug that made this look "already aligned" while it wasn't.
    //
    // ResizeObserver — on relayout xterm re-fits to a new column count, so the slack
    // changes. Measured a frame later, because the observer fires when the CONTAINER
    // resizes and xterm re-fits after that; same-tick reads the stale canvas.
    let frame = 0
    const remeasure = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }

    const resize = new ResizeObserver(remeasure)
    resize.observe(column)

    const mutation = new MutationObserver(remeasure)
    mutation.observe(column, { childList: true, subtree: true })

    remeasure()

    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [attachment.state, presentation])

  // Re-point the buffer at what this pane is actually showing. This is the write that
  // makes the tab follow: pane-container feeds the buffer's chatId/runnerId straight
  // back in as our props, so the next render is already looking at the new chat.
  //
  // It converges in one step and cannot loop — once the buffer holds the pair computed
  // here, the effect's deps are unchanged and it does not re-run. Writing '' as the
  // runnerId of a dormant chat is deliberate: a tab must not go on claiming a runner
  // that no longer exists.
  useEffect(() => {
    if (!known) return // nothing authoritative to re-point at yet
    if (shownChatId === chatId && liveRunnerId === runnerId) return
    store.getState().bufferActions.repointAgentChatBuffer(bufferId, {
      chatId: shownChatId,
      runnerId: liveRunnerId,
    })
  }, [store, bufferId, known, chatId, runnerId, shownChatId, liveRunnerId])

  // The tab label is a snapshot taken by openContent, but the title changes AFTER the
  // tab opens: the agent auto-titles the chat (WS `title_set`), the user renames it from
  // the sidebar — and the tab may now be showing a DIFFERENT chat than it opened on,
  // which has a title of its own. All three land on the shown chat's `title`, so
  // mirroring title → buffer name here keeps the tab honest for all of them, with no
  // store → component dependency.
  //
  // An untitled chat is NOT necessarily a tab that needs renaming — and the difference
  // is the whole subtlety here. A tab opens carrying a provider placeholder ("Codex
  // chat"), which is a perfectly good name for a chat nobody has titled yet, and blanking
  // that would be a regression.
  //
  // The tab is only WRONG when it is wearing ANOTHER CHAT'S TITLE, and that happens in
  // exactly one situation: the runner MOVED us to a fresh chat (a /clear), which has no
  // title of its own to overwrite the old one with. Bailing on `!title` left the tab
  // reading "reply with exactly: ORION" while showing a conversation that had nothing to
  // do with it — caught by running it, not by a test, because the sibling test moves the
  // runner into a chat that HAS a title.
  //
  // So: a title always names the tab; an empty title only renames it if we have just
  // arrived from somewhere else.
  const labelledFor = useRef('')
  useEffect(() => {
    const s = store.getState()
    const buffer = s.bufferActions.getBufferById(bufferId)
    if (!buffer) return

    const arrivedFromAnotherChat = labelledFor.current !== '' && labelledFor.current !== shownChatId
    labelledFor.current = shownChatId

    const label = title || (arrivedFromAnotherChat ? UNTITLED_CHAT_LABEL : buffer.name)
    if (buffer.name !== label) s.bufferActions.renameBuffer(bufferId, label)
  }, [store, bufferId, title, shownChatId])

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

  // adopt reads the chat back and settles the pane on what the SERVER says: attach to the
  // runner now on it, or report that there still is none (false). It only ever READS, so
  // it cannot spawn anything; the caller has already done the spawning.
  //
  // Reading is not belt-and-braces. The WS would push the same facts eventually, but this
  // way the pane settles on the ACT rather than whenever a frame happens to arrive — and,
  // crucially, it settles AT ALL: a CLI that died on startup leaves the chat dormant, and
  // this read says so, where waiting for a frame that is never coming would spin forever.
  const adopt = useCallback(async (): Promise<boolean> => {
    const chat = await getChat(wsId, shownChatId)
    const s = store.getState()
    s.upsertAgentChat(chat)
    s.setAgentChatWorking(chat.id, chat.working === true)
    if (!chat.liveRunnerId || !chat.terminalSessionId) return false
    s.bufferActions.repointAgentChatBuffer(bufferId, {
      chatId: chat.id,
      runnerId: chat.liveRunnerId,
    })
    seedAttach(wsId, chat.terminalSessionId)
    setAttachment({ state: 'attached', sessionId: chat.terminalSessionId })
    return true
  }, [store, wsId, bufferId, shownChatId])

  // Re-check the aggregate after a prompt race. The prompt queue consumes only
  // this server-folded value; it never guesses busy state from a lifecycle kind.
  const refreshChatWorking = useCallback(async (): Promise<boolean> => {
    const chat = await getChat(wsId, shownChatId)
    const s = store.getState()
    s.upsertAgentChat(chat)
    s.setAgentChatWorking(chat.id, chat.working === true)
    return chat.working === true
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
    if (!sessionId) {
      if (switchingRef.current) return // the switch's own spinner stands
      // ONLY THE VISIBLE TAB REVIVES. A dormant chat kept alive on a hidden tab must
      // NOT spawn a CLI: opening a workspace with N dormant chat tabs would otherwise
      // fire N revives at once, one CLI per hidden tab. A hidden dormant chat waits;
      // when the user switches to it (isVisible flips true, this effect re-runs) it
      // revives — exactly once, because the budget below is spent inside revive(). An
      // already-attached chat has a sessionId and never reaches here, so it keeps its
      // live PTY while hidden.
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
    seedAttach(wsId, sessionId)
    setAttachment({ state: 'attached', sessionId })
  }, [wsId, known, sessionId, shownChatId, revive, isVisible])

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
  const handleSwitch = (providerId: string) => {
    if (
      switchingRef.current ||
      promptReplacing ||
      deliveryPending ||
      attachment.state === 'reviving'
    )
      return
    const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
    // Held across the whole request: the outgoing CLI is killed FIRST, so this window is
    // exactly the transient dormancy — and its dead PTY's onSessionGone — that nothing
    // must mistake for a chat needing revival.
    switchingRef.current = true
    setAttachment({ state: 'reviving', message: `Starting ${name}…` })
    void (async () => {
      try {
        await switchProvider(wsId, shownChatId, providerId)
        if (!(await adopt())) fail()
      } catch (err: unknown) {
        fail()
        // Status-aware: only a 424 actually means "that CLI is not installed". Blaming the
        // PATH for every failure sends the user hunting for a problem they do not have.
        toastSpawnFailure(err, name, 'switch to')
      } finally {
        switchingRef.current = false
      }
    })()
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
    handleSwitch(next.id)
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
      if (presentation === 'terminal' || !userIsWatching()) return
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
  const chooseSurface = (next: 'chat' | 'terminal') => {
    if (escortRef.current !== 'none') escortRef.current = next === 'chat' ? 'none' : 'released'
    if (next === 'chat') setReturnOffered(false)
    setPresentation(next)
  }

  // The banner's own button. They are going because Crowbar asked them to, so it
  // owes them the way back — but it did not move them, so it must not move them
  // back unasked either. 'released' is exactly that: offer, never take.
  const openTerminalFromBanner = () => {
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
        className="mx-auto flex min-h-0 w-full max-w-4xl flex-1 flex-col px-4 pt-4"
      >
        <div className="relative min-h-0 flex-1">
          {/* Both presentations stay mounted. Keeping the queue mounted is what makes
              a Terminal detour non-destructive; keeping xterm mounted preserves its
              screen model while Chat is in front. Only the selected surface is active. */}
          <div className={cn('h-full', presentation === 'chat' ? '' : 'hidden')}>
            <AgentChatView
              key={`${wsId}:${shownChatId}`}
              ref={chatViewRef}
              wsId={wsId}
              chatId={shownChatId}
              providerId={activeProviderId}
              providers={providers}
              working={working}
              turnRevision={turnRevision}
              live={attachment.state === 'attached' || promptReplacing}
              active={presentation === 'chat'}
              visible={isVisible}
              onOpenTerminal={() => setPresentation('terminal')}
              terminalWaiting={waiting}
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
            />

            {attachment.state === 'reviving' && (
              <div className="absolute inset-x-4 top-2 flex items-center justify-center gap-2 rounded-lg border bg-popover/95 px-3 py-2 text-muted-foreground text-sm shadow-sm">
                <FlickerSpinner className="size-4 text-foreground" />
                {attachment.message}
              </div>
            )}
            {attachment.state === 'idle' && (
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
                  onClick={() => void revive()}
                >
                  Resume
                </Button>
              </div>
            )}
            {waiting && (
              // The pane's own overlay slot, the one `reviving` and `idle` use —
              // this is the same class of statement as those two, about the chat
              // rather than about anything in the transcript.
              //
              // The opaque bg is load-bearing under it: the warning card is a
              // TINT, and tinting the messages behind it would make both
              // unreadable.
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

          <div className={cn('h-full', presentation === 'terminal' ? '' : 'hidden')}>
            {attachment.state === 'attached' && (
              // NO key={sessionId}. Runner replacement swaps the PTY imperatively
              // instead of rebuilding xterm; runner movement keeps the same PTY.
              <XtermTerminal
                sessionId={attachment.sessionId}
                workspaceId={wsId}
                isActive={isActivePane && isVisible && presentation === 'terminal'}
                isVisible={isVisible && presentation === 'terminal'}
                attachOnly
                flush
                onTerminalRef={(api) => {
                  terminalApiRef.current = api
                }}
                onSessionGone={handleSessionGone}
              />
            )}
            {presentation === 'terminal' && attachment.state === 'reviving' && (
              <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
                <FlickerSpinner className="size-6 text-foreground" />
                <p className="text-muted-foreground text-center text-sm">{attachment.message}</p>
              </div>
            )}
            {presentation === 'terminal' && attachment.state === 'idle' && (
              <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
                <p className="text-muted-foreground max-w-sm text-center text-sm">
                  {attachment.reason === 'failed'
                    ? 'Crowbar could not restart this agent. Check that its CLI is installed, then try again — or pick another provider below.'
                    : 'This agent has exited. Resume it to pick the conversation up where you left off.'}
                </p>
                <Button type="button" variant="secondary" size="sm" onClick={() => void revive()}>
                  Resume
                </Button>
              </div>
            )}
          </div>
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

        {/* The chat's own status line, spanning the terminal's width: what this
            conversation IS on the left, who is running it on the right. Both sit on the
            column, so the title starts on the agent's first character and the switcher
            ends on its last one.
            min-w-0 is what lets a long title truncate instead of shoving the switcher
            off the column — a flex child defaults to min-width:auto and refuses to
            shrink below its content. */}
        <div
          className="flex items-center justify-between gap-3 py-2"
          style={{ paddingRight: gridSlack }}
        >
          <span className="min-w-0 truncate text-muted-foreground text-sm">{title}</span>
          <div
            className="flex shrink-0 items-center rounded-lg bg-muted p-0.5"
            aria-label="Presentation"
          >
            <Button
              size="xs"
              variant={presentation === 'chat' ? 'secondary' : 'ghost'}
              aria-pressed={presentation === 'chat'}
              onClick={() => chooseSurface('chat')}
            >
              <MessageSquareIcon /> Chat
            </Button>
            <Button
              size="xs"
              variant={presentation === 'terminal' ? 'secondary' : 'ghost'}
              aria-pressed={presentation === 'terminal'}
              onClick={() => chooseSurface('terminal')}
            >
              <TerminalIcon /> Terminal
            </Button>
          </div>
          <ProviderSwitchDropdown
            providers={providers}
            currentProviderId={activeProviderId}
            onSwitch={handleSwitch}
            disabled={promptReplacing || deliveryPending || attachment.state === 'reviving'}
          />
        </div>
      </div>
    </div>
  )
}

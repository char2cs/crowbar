import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useStore } from 'zustand'
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
import { ProviderSwitchDropdown } from './provider-switch-dropdown'

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
  const title = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.title ?? '',
  )
  const providers = useStore(store, (s) => s.agentChats.providers)
  // The provider this chat last ran under — what a failed revive has to NAME ("Claude
  // isn’t installed"), since a dormant chat has no live runner to ask.
  const chatProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.activeProviderId ?? '',
  )

  const [attachedState, setAttachment] = useState<Attachment>({ state: 'pending' })
  // `pending` while the chat list is still in flight is DERIVED, not written by
  // the attach effect below. Dormancy is unknowable until the list lands, so
  // there is nothing for the machine to record — the pane simply has nothing to
  // show yet. Keeping it out of the state means the effect's only job is
  // spawning and attaching CLIs, and a `known` flip no longer costs an extra
  // render to undo a value the effect had just written.
  const attachment: Attachment = known ? attachedState : { state: 'pending' }

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
    if (attachment.state !== 'attached' || !column) {
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
  }, [attachment.state])

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
    if (!chat.liveRunnerId || !chat.terminalSessionId) return false
    s.bufferActions.repointAgentChatBuffer(bufferId, {
      chatId: chat.id,
      runnerId: chat.liveRunnerId,
    })
    seedAttach(wsId, chat.terminalSessionId)
    setAttachment({ state: 'attached', sessionId: chat.terminalSessionId })
    return true
  }, [store, wsId, bufferId, shownChatId])

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
  const handleSessionGone = useCallback(() => {
    if (switchingRef.current) return // our own switch killing the outgoing CLI: expected
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
    if (attachment.state !== 'attached') return
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
        <div className="min-h-0 flex-1">
          {attachment.state === 'attached' && (
            // NO key={sessionId}. When the agent's runner is replaced under a still-
            // attached pane (a provider switch, or a revive that lands a new CLI), the
            // sessionId changes but the terminal stays mounted: XtermTerminal swaps the
            // PTY imperatively — detach the old, attach the new — instead of tearing the
            // whole component (socket, observers, xterm) down and rebuilding it. A move
            // (same PTY, new conversation) never changed the id and so never remounted;
            // this makes a REPLACEMENT behave the same way.
            <XtermTerminal
              sessionId={attachment.sessionId}
              // The chat's own workspace: a transport-drop reconnect on a hidden
              // keep-alive workspace must listLive/attach against THIS workspace,
              // or the agent's live PTY looks gone from the active one's list.
              workspaceId={wsId}
              // isActive = focused AND the visible tab; isVisible = the visible tab.
              // A hidden keep-alive chat is neither, so its terminal fits locally but
              // does not steal focus or (Task 4) push PTY resize while off-screen.
              isActive={isActivePane && isVisible}
              isVisible={isVisible}
              attachOnly
              // The column already supplies the inset; the terminal's own pl-[16px]
              // would double it and push the agent out of line with the switcher.
              flush
              onTerminalRef={(api) => {
                terminalApiRef.current = api
              }}
              onSessionGone={handleSessionGone}
            />
          )}
          {attachment.state === 'reviving' && (
            <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
              <FlickerSpinner className="size-6 text-foreground" />
              <p className="text-muted-foreground text-center text-sm">{attachment.message}</p>
            </div>
          )}
          {/* The dormant states — and the ONLY place the Resume button lives. Opening a
              chat no longer lands here: a dormant chat is revived on sight, so reaching
              this means either the revive FAILED, or the CLI died a second time and this
              pane has stopped bringing it back unasked. The button is the user's way in
              from both; so is the provider dropdown below it. */}
          {attachment.state === 'idle' && (
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
          <ProviderSwitchDropdown
            providers={providers}
            currentProviderId={activeProviderId}
            onSwitch={handleSwitch}
          />
        </div>
      </div>
    </div>
  )
}

import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { listChats, getChat, listProviders } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { toast } from '@/features/window/stores/toast-store'

type WorkspaceSnapshot = ReturnType<WorkspaceStore['getState']>

// closeTab closes a pane tab the way the tab's own × button does: remove it from
// every pane holding it FIRST, then drop the buffer.
//
// Raw closeBuffer is NOT enough, and the shortcut cost a live bug: it only filters
// the buffer out of the buffers array, leaving the pane's activeBufferId pointing
// at a buffer that no longer exists. Deleting the chat you were looking at then
// blanked the whole pane — the remaining tab was still in the tab bar, but the
// pane rendered its empty "New Terminal" state until you clicked that tab.
// pane-slice's removeBufferFromPane is what activates an adjacent tab instead.
function closeTab(st: WorkspaceSnapshot, bufferId: string): void {
  for (const pane of Object.values(st.panes ?? {})) {
    if (pane.bufferIds.includes(bufferId)) st.paneActions.removeBufferFromPane(pane.id, bufferId)
  }
  st.bufferActions.closeBuffer(bufferId)
}

function closeChatTab(st: WorkspaceSnapshot, chatId: string): void {
  const buf = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === chatId)
  if (buf) closeTab(st, buf.id)
}

// Where is this runner? Two independent answers, and we want the first that exists:
//
//   the CHAT LIST — the server's own placement, seeded and refetched. A chat is live
//     exactly while a runner sits on it, so the chat claiming `runnerId` IS the
//     runner→chat mapping, held for us, for every runner (not just the tabbed ones).
//   the TAB — where the client last saw it. The fallback matters on a cold client
//     whose chat list has not landed yet, or for a runner on a chat the list has
//     since replaced: a tab still following it is evidence.
function chatOfRunner(st: WorkspaceSnapshot, runnerId: string): string {
  const claimed = st.agentChats.chats.find((c) => c.liveRunnerId === runnerId)
  if (claimed) return claimed.id
  const tab = st.buffers.find((b) => b.type === 'agentChat' && b.runnerId === runnerId)
  return tab && tab.type === 'agentChat' ? tab.chatId : ''
}

// The display name of the provider currently on `chatId`. Read BEFORE the chat is
// refetched — an arriving runner overwrites activeProviderId with its own, and the
// name of the CLI that just got closed is then gone for good.
function providerOn(st: WorkspaceSnapshot, chatId: string): string {
  const providerId = st.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? ''
  return st.agentChats.providers.find((p) => p.id === providerId)?.displayName ?? 'The agent'
}

// One wire frame on the workspace-scoped agent feed. TWO vocabularies ride it:
//
//   CHAT frames    — created / turn_started / turn_stopped / title_set / session_bound /
//                    deleted. About the conversation. They name no process.
//   RUNNER frames  — started / session_bound / moved / displaced / exited. About the
//                    vendor-CLI PROCESS, which is a thing that moves between chats.
//
// runnerId IS THE DISCRIMINATOR, and it is not an optimisation to be tidied away:
// `session_bound` exists in BOTH vocabularies, so kind alone is ambiguous and would
// misroute. runnerId is `omitempty` on the wire and chat frames never set it, so a
// frame is a runner frame *iff* runnerId is present. That is structural, not temporal.
//
// The frames carry no snapshot (00 agentic-engine spec §7), so most kinds
// react-then-refetch; only turn_started/turn_stopped say enough ON THE FRAME to update
// the store without a round trip — they carry `working`. A round trip is not an option
// for those two: they are the hottest frames on the feed and the spinner has to be right
// the instant the frame lands.
interface AgentStreamEvent {
  /** The chat the frame is about — EMPTY on `displaced`, where the emptiness is the point. */
  chatId: string
  workspaceId: string
  kind:
    | 'created'
    | 'session_bound'
    | 'turn_started'
    | 'turn_stopped'
    | 'title_set'
    | 'deleted'
    | 'started'
    | 'moved'
    | 'displaced'
    | 'exited'
  /** Set only on runner frames. Present ⟺ this frame is about a process. */
  runnerId?: string
  /**
   * The chat's folded busy state as of this event, straight from the aggregate
   * (domain.AgentChat.Working) — the server's answer to the spinner.
   *
   * Never recomputed here from the kind. `turn_stopped` does NOT mean idle: a CLI that
   * handed work to a background subagent has genuinely ended its turn and is still
   * working, and a client that reads "turn stopped" as "done" goes dark on a live agent.
   * That is the bug this field exists to end.
   *
   * Meaningful on the chat kinds; runner frames name a process, not a conversation, and
   * never reach the branch that reads it.
   */
  working?: boolean
}

/**
 * Subscribe to the workspace-scoped agent WS while `wsId` is active. Seed via GET,
 * subscribe, reseed on the {reconnected} sentinel, and route each frame:
 *
 *  CHAT frames
 *   - turn_started / turn_stopped: the frame carries the server's folded `working`
 *     — write it through, no refetch. `turn_stopped` is NOT "idle": a chat waiting on
 *     a background subagent keeps spinning through it.
 *   - created: a new chat (and its ordering) may have appeared — reseed the whole list.
 *   - title_set / session_bound: refetch just that chat and upsert it.
 *   - deleted: drop the chat from the store and close its pane tab if open.
 *
 *  RUNNER frames — the tab is a viewport on a MOVING TARGET, and this is what moves it.
 *   - moved: the CLI switched conversation (the user typed /clear or /resume inside it).
 *     Re-point the tab that follows it, evict a tab that already held the destination,
 *     and invalidate BOTH chats.
 *   - displaced: Crowbar took the CLI off its chat. Let go of it at once.
 *   - started / session_bound / exited: refetch the chat named, and let the pane
 *     re-resolve (a chat nobody is on renders dormant + Resume).
 */
export function useWorkspaceAgentChatsStream(wsId: string): void {
  useEffect(() => {
    let cancelled = false

    const stateOf = () => getOrCreateWorkspaceStore(wsId).getState()

    // Every single-chat read that lands bumps this. A list seed captures it before it
    // asks, and refuses to apply a snapshot that a fresher read has already overtaken —
    // see seedChats.
    let chatWrites = 0

    // The seed is a full RECONCILE, not a merge: it runs on first load AND on every
    // reconnect, and on reconnect it is the only thing that can repair frames the
    // socket dropped while it was down. seedAgentChats therefore drops chats the
    // server no longer has (a missed `deleted`) and clears the working map (a missed
    // `turn_stopped` must not strand a spinner; working is unknown here → idle).
    //
    // Being a REPLACE is exactly why it must not land out of order. A CLI walking into a
    // conversation Crowbar has never seen is TWO backend writes on TWO aggregates — mint
    // the chat, move the runner — and therefore two frames: `created`, then `moved`. This
    // seed is the one `created` fires, so its request goes out FIRST, and the daemon can
    // serve it in the window before the move is projected: the snapshot then shows the new
    // chat with NO RUNNER. The `moved` frame's single-chat read is issued second, reads the
    // truth, and lands first. If this stale snapshot is then applied on top, it overwrites
    // the live chat with the dormant snapshot of itself — and since the tab has by now
    // FOLLOWED the runner into that chat, the pane renders "this agent has exited" over a
    // CLI that is alive and typing. Nothing refetches afterwards, so it stays that way.
    //
    // So: a snapshot overtaken by a fresher read is DISCARDED, and we simply ask again —
    // the next read is taken after those writes and is consistent with them. The retry is
    // bounded because it is not a fix for contention, only for order: the losing case needs
    // a per-chat read to land inside this request's flight, and each attempt is one more
    // chance for that to have stopped happening. If it somehow never settles we leave the
    // list alone rather than knowingly writing stale data over fresh — the per-chat reads
    // have already put the truth in the store; only the reconcile is skipped.
    const seedChats = async () => {
      for (let attempt = 0; attempt < 3; attempt++) {
        try {
          const issuedAt = chatWrites
          const chats = await listChats(wsId)
          if (cancelled) return
          if (chatWrites !== issuedAt) continue // overtaken in flight — this snapshot is old news

          const store = getOrCreateWorkspaceStore(wsId)
          const before = store.getState()
          before.hydrateAgentChatOrder()

          const present = new Set(chats.map((c) => c.id))
          const vanished: string[] = []
          for (const c of before.agentChats.chats) {
            if (!present.has(c.id)) vanished.push(c.id)
          }

          store.getState().seedAgentChats(chats)

          // A chat deleted during the outage never delivered its `deleted` frame, so
          // close its pane tab here exactly as that frame's handler would have.
          for (const chatId of vanished) closeChatTab(store.getState(), chatId)
          return
        } catch {
          return /* seed failure is non-fatal — the WS stream still pushes */
        }
      }
    }

    const seedProviders = async () => {
      try {
        const providers = await listProviders(wsId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().setAgentProviders(providers)
      } catch {
        /* non-fatal */
      }
    }

    // Returns whether the chat actually landed in the store, so a caller that is about
    // to re-point a tab at it can decline when it did not: a tab pointed at a chat the
    // store has never heard of renders nothing at all.
    //
    // This is a POINT-IN-TIME read of ONE chat, and it outranks any list snapshot taken
    // before it — hence the chatWrites bump, which is what lets seedChats know it has been
    // overtaken (see there).
    const refetchOne = async (chatId: string): Promise<boolean> => {
      try {
        const chat = await getChat(wsId, chatId)
        if (cancelled) return false
        getOrCreateWorkspaceStore(wsId).getState().upsertAgentChat(chat)
        chatWrites++
        return true
      } catch {
        /* a not-found here is handled by the deleted frame path */
        return false
      }
    }

    // The runner has arrived on `entered`. Make the tabs agree with that.
    //
    // Runs AFTER `entered` has been refetched, deliberately: the store then already says
    // the runner is there, so this write and the pane's own follow rule ("follow my
    // runner if it still exists anywhere") compute the same pair and converge in one
    // step instead of fighting each other across a round trip.
    //
    // And it must run even when no pane is mounted for the tab: a tab in the background
    // of a split has no AgentChatPane rendering, so nothing else would ever re-point it.
    const followRunner = (runnerId: string, entered: string, closedProvider: string) => {
      const st = stateOf()
      const taker = st.buffers.find((b) => b.type === 'agentChat' && b.runnerId === runnerId)
      if (!taker) return // no tab follows this runner — the chat list update is the whole story

      // ONE TAB PER LIVE CONVERSATION. If another tab was already showing the chat this
      // runner has just walked into, its own CLI was pushed off it (that is what an
      // eviction IS) and it now shows a conversation it does not own. Close it and leave
      // the user looking at the tab that took the conversation over. A terminal tab holds
      // no unsaved state, so closing it costs nothing.
      const evicted = st.buffers.filter(
        (b) => b.type === 'agentChat' && b.chatId === entered && b.id !== taker.id,
      )
      for (const b of evicted) closeTab(st, b.id)

      st.bufferActions.repointAgentChatBuffer(taker.id, { chatId: entered, runnerId })
      if (evicted.length === 0) return

      // Re-read: closeTab moved panes underneath us (removeBufferFromPane activates an
      // adjacent tab), so the pre-close snapshot's panes are stale.
      const after = stateOf()
      const pane = Object.values(after.panes ?? {}).find((p) => p.bufferIds.includes(taker.id))
      if (pane) after.paneActions.activatePaneBuffer(pane.id, taker.id)

      // Told, not silently done: a tab vanished and another one changed what it is
      // showing. Fired from here — a hook, i.e. component-scope code — and never from a
      // slice; `toast` is the imperative toast API, not a component (CLAUDE.md).
      toast.info(
        'Conversation moved',
        `${closedProvider} was closed — that conversation is now in this terminal.`,
      )
    }

    const onRunnerFrame = (ev: AgentStreamEvent & { runnerId: string }) => {
      const st = stateOf()

      switch (ev.kind) {
        case 'moved': {
          const entered = ev.chatId
          if (!entered) return // a move always names its destination

          // ⚠️ A `moved` frame names ONLY the chat ENTERED. The chat LEFT is named by
          // NOTHING — so if we key off ev.chatId alone, that chat goes on advertising a
          // live runner that has gone. Two chats then claim one runner, a pane following
          // it resolves to the stale claimant, and the tab never follows. Which is the
          // original bug, unfixed. We know the chat it left, because the chat list holds
          // the runner→chat mapping — so read it BEFORE the refetch overwrites it.
          const left = chatOfRunner(st, ev.runnerId)
          const closedProvider = providerOn(st, entered) // see providerOn: read before, or lost

          void (async () => {
            const landed = await refetchOne(entered)
            if (cancelled) return
            if (landed) followRunner(ev.runnerId, entered, closedProvider)
            if (left && left !== entered) void refetchOne(left)
          })()
          return
        }

        case 'displaced': {
          // ev.chatId is EMPTY here, and that emptiness IS the frame's meaning: Crowbar
          // has taken this CLI OFF its chat (an eviction, the outgoing half of a provider
          // switch, a chat deleted under it). It holds nothing now.
          //
          // The process may still be alive — displacement asserts nothing about liveness —
          // so DO NOT wait for `exited` before letting go: if the kill failed, `exited`
          // never comes, and the pane stays welded to a runner that owns nothing.
          //
          // Idempotent, and safe for a runner we have never seen: the second frame finds
          // no tab following it and no chat claiming it, and does nothing.
          const held = chatOfRunner(st, ev.runnerId)
          for (const b of st.buffers) {
            if (b.type !== 'agentChat' || b.runnerId !== ev.runnerId) continue
            // The tab keeps its chat and lets the runner go. The pane's second rule then
            // takes over — adopt whoever is on my chat — so it picks up a successor if one
            // arrives, and renders dormant + Resume if none does.
            st.bufferActions.repointAgentChatBuffer(b.id, { chatId: b.chatId, runnerId: '' })
          }
          // Re-read the chat it held NOW rather than on some later frame: whether it went
          // dormant or was taken over, the answer is already true on the server.
          if (held) void refetchOne(held)
          return
        }

        default:
          // started | session_bound | exited. Placement changed on the chat named, and
          // the chat list is the whole answer — the pane re-resolves from it.
          //
          // An `exited` that follows a `displaced` carries NO chat (the runner was already
          // on none by the time it died), and there is nothing to re-read: `displaced`
          // already let go of it.
          if (ev.chatId) void refetchOne(ev.chatId)
      }
    }

    void seedChats()
    void seedProviders()

    const unsubscribe = wsManager.subscribe(`${workspaceBase(wsId)}/agent/ws/chats`, (frame) => {
      if (cancelled) return
      // Reconnect sentinel emitted by the manager after a socket drop+reopen —
      // reseed so pushes missed during the outage aren't lost.
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        void seedChats()
        return
      }
      const ev = frame as AgentStreamEvent
      // The discriminator, and the first branch for that reason — see AgentStreamEvent.
      if (ev.runnerId) {
        onRunnerFrame({ ...ev, runnerId: ev.runnerId })
        return
      }
      if (!ev.chatId) return
      const st = stateOf()
      switch (ev.kind) {
        case 'turn_started':
        case 'turn_stopped':
          // The FRAME says whether the chat is working; this does not decide. Both kinds
          // go through the same line on purpose — `turn_stopped` is not "idle" and
          // `turn_started` is not "busy", they are just the two moments the answer can
          // change, and the answer itself was folded by the aggregate that emitted them.
          //
          // Hardcoding false here is exactly what kept the spinner dark under a live
          // background subagent even after the server knew better.
          st.setAgentChatWorking(ev.chatId, ev.working === true)
          return
        case 'deleted': {
          st.removeAgentChat(ev.chatId)
          closeChatTab(st, ev.chatId)
          return
        }
        case 'created':
          void seedChats() // new chat + ordering — reseed the whole list
          return
        default:
          void refetchOne(ev.chatId) // title_set / session_bound
      }
    })

    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId])
}

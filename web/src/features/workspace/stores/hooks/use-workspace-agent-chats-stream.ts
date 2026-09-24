import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import {
  chatBase,
  listChats,
  getChat,
  listProviders,
  listChatFolders,
  mapChat,
  type AgentChat,
  type AgentTelemetry,
} from '@/features/agent/api/agent-api'
import {
  runnerLeft,
  startedWorking,
  type ChatFrameOutcome,
} from '@/features/agent/lib/reduce-chat-frame'
import { createStreamingMessageBatcher } from '@/features/workspace/stores/hooks/lib/streaming-message-batcher'
import { getWorkspaceScope, useWorkspaceScopeReady } from '@/lib/workspace-scope'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import {
  getOrCreateWorkspaceStore,
  resolveChatOwnerWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { isNotFoundError } from '@/lib/api'
import {
  isLatestProviderWrite,
  providerWriteGeneration,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { toast } from '@/features/window/stores/toast-store'

type WorkspaceSnapshot = ReturnType<WorkspaceStore['getState']>

/** Whether the "could not load agent providers" toast is already on screen for
 *  the current outage. MODULE scope, not per-hook: the provider list is
 *  machine-level and every mounted workspace's copy of this hook fails at the
 *  same moment for the same reason. Cleared by the next successful read. */
let providersUnreachableAnnounced = false

/** Test-only: module state outlives a `renderHook`, so a suite with a
 *  deliberate provider outage in one case would silence the toast in every
 *  later one. Same shape as `_resetTerminalFocusRegistryForTests`. */
export function _resetProviderToastForTests(): void {
  providersUnreachableAnnounced = false
}

/**
 * The chat frames that say NOTHING about the tree's shape.
 *
 * Listed as the exception rather than listing the structural kinds, and that
 * direction is the point: every one of these is either a turn in flight or a
 * question about which PROCESS is on a chat, and both sets are closed and
 * well-known. Everything else a chat aggregate can emit — created, deleted,
 * title_set, placement_set, order_set, and any placement kind a newer daemon
 * mints — has moved, renamed or removed a ROW, and the sidebar has to be told.
 * Defaulting the unknown kind to "the tree moved" costs one repo-scoped reseed;
 * defaulting it the other way is a row that silently never appears, which is
 * exactly how folders came to not sync across windows.
 *
 * `turn_started`/`turn_stopped`/`message_delta` in particular MUST stay out of
 * the structural set: they are the hottest frames on the feed, and reseeding a
 * whole repo's chat list on each one is a request storm per agent turn.
 */
export const NON_STRUCTURAL_CHAT_KINDS: ReadonlySet<string> = new Set([
  'turn_started',
  'turn_stopped',
  'message_delta',
  'terminal_wait',
  'prompt_settled',
  'session_bound',
  // A snapshot republished for a change no aggregate event names (a phase, an
  // attach, a terminal-wait verdict), and the runner lifecycle: each moves a
  // process, never a row.
  'snapshot',
  'started',
  'moved',
  'displaced',
  'exited',
  // The live views of a turn in progress: the plan, the compaction edge and the
  // usage gauge. Each rides its whole payload and moves no row; treating them
  // as structural reseeded a repo's chat list on every plan restatement.
  'plan',
  'compaction_started',
  'compaction_stopped',
  'telemetry',
  // `worktree_state` belongs here for exactly the reason the three hot kinds
  // above do. It carries the git state of the worktree a chat owns — diff
  // counts, PR state, lock status — and it is emitted from the same push site
  // as every workspace frame, so it fires on each working-tree sync and each
  // provider poll. It moves no row: the frame names a chat that already exists
  // and changes only what is drawn ON it, and the whole payload rides the frame
  // (see AgentChatEvent.Worktree), so there is nothing to re-read. Treating it
  // as structural would reseed a whole repo's chat list every time somebody
  // saved a file.
  'worktree_state',
])

// The display name of the provider currently on `chatId`. Read BEFORE the
// frame is applied — an arriving runner overwrites activeProviderId with its
// own, and the name of the CLI that just got closed is then gone for good.
function providerOn(st: WorkspaceSnapshot, chatId: string): string {
  const providerId = st.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? ''
  return st.agentChats.providers.find((p) => p.id === providerId)?.displayName ?? 'The agent'
}

// One wire frame on the agent feed (chatBase(wsId)/ws — repo-scoped, or the
// /home mount for a project-home workspace; see agent-api.ts's chatBase).
//
// EVERY chat and runner lifecycle frame carries the chat's whole versioned
// snapshot (`chat`, `version`), built by the daemon's one snapshot owner under
// the lock that assigns the version. So the client applies it by one rule —
// newer version wins (reduce-chat-frame.ts) — and never refetches a chat, never
// guesses from the kind, and never orders reads by when it asked.
//
// The frames that carry NO snapshot are the live views of a turn in progress
// (message_delta, plan, telemetry, compaction_*, prompt_settled), the worktree
// state, the folder frames, and the delete (which carries only its version).
interface AgentStreamEvent {
  /** The chat the frame is about. */
  chatId: string
  workspaceId: string
  kind: string
  /** The runner a runner-lifecycle frame was about (started/moved/displaced/…). */
  runnerId?: string
  /** Set only on folder frames. Present ⟺ this frame is about the tree. */
  folderId?: string
  /** The chat's whole snapshot, and the version that orders it. */
  chat?: AgentChat
  version?: number
  /** The client request id of one prompt that is over, on `prompt_settled`. */
  clientRequestId?: string
  /** Whether anything proved the provider took that prompt — see
   *  AgentChatsState.settledPrompts/abandonedPrompts. Absent reads as false,
   *  which is the answer that preserves the user's text. */
  promptConsumed?: boolean
  /** An assistant message still being produced, on `message_delta` — the text
   *  SO FAR. `kind` is absent for the answer, `reasoning` for a thought,
   *  `tool_output` for a running tool's output. */
  message?: { id: string; text: string; kind?: string }
  /** The agent's own running to-do list, on `plan` — always the whole list. */
  plan?: { text: string; status: string }[]
  /** The provider's newest usage report, on `telemetry`. */
  telemetry?: AgentTelemetry
}

/**
 * Subscribe to the agent WS for `wsId`'s chat scope. Seed via GET, subscribe,
 * reseed on the {reconnected} sentinel, and apply each frame through the one
 * reducer. What remains here is the EFFECTS an applied snapshot implies — a pane
 * following its runner, a background record for a chat that starts working with
 * no pane — and the live views that are not chat state at all.
 */
export function useWorkspaceAgentChatsStream(wsId: string): void {
  // `chatBase(wsId)` below (agent-api.ts) throws the instant project/repo scope
  // is missing entirely. This hook runs for EVERY mounted workspace, so it is
  // exactly the effect a force-mounted, never-navigated-to workspace hits first.
  // Wait rather than crash; see useWorkspaceScopeReady's own doc.
  const scopeReady = useWorkspaceScopeReady(wsId)
  useEffect(() => {
    if (!scopeReady) return
    let cancelled = false

    const stateOf = () => getOrCreateWorkspaceStore(wsId).getState()

    // Tells the sidebar's per-repo TREE subscription that THIS repo's tree may
    // have moved. Scoped to this workspace's own repo, which is the cross-repo
    // guard: a chat frame for repo A can never reseed repo B.
    const bumpTreeSignal = () => {
      const repoId = getWorkspaceScope(wsId)?.repoId
      if (repoId) useFolderSignalStore.getState().bump(repoId)
    }

    // `message_delta` fires once per streamed token, each its own top-level WS
    // callback — outside anything React batches. This collapses them into one
    // store write per chat per frame.
    const streamingMessages = createStreamingMessageBatcher((chatId, message) =>
      stateOf().setAgentChatStreamingMessage(chatId, message),
    )

    // The list read. Every row is a versioned snapshot applied under the same
    // rule a frame is, so it can land in any order relative to the feed: a row
    // older than what a frame already delivered is simply not applied. No
    // sequencing, no retry, no "keep working" exception.
    const seedChats = async () => {
      try {
        const chats = await listChats(wsId)
        if (cancelled) return
        const vanished = stateOf().seedAgentChats(chats)
        // A chat missing from the list is only a SUSPECT: a repo-scoped list
        // can omit a project-home chat that still exists, and a chat created
        // after the list was served is newer than it. Forget it only on a
        // definite not-found; the `deleted` frame forgets directly.
        for (const id of vanished) void forgetIfGone(id)
      } catch {
        /* non-fatal: the WS stream still pushes every change */
      }
    }

    // Only the chat's own workspace may confirm it gone: stores hold other
    // workspaces' chats, and asking through the wrong mount 404s a live chat.
    const forgetIfGone = async (chatId: string) => {
      const held = stateOf().agentChats.chats.find((c) => c.id === chatId)
      const owner = held?.workspaceId || resolveChatOwnerWorkspaceId(chatId)
      if (owner !== wsId) return
      try {
        stateOf().applyAgentChat(await getChat(owner, chatId))
      } catch (err) {
        if (cancelled || !isNotFoundError(err)) return
        stateOf().removeAgentChat(chatId)
        windowPaneStore.getState().paneActions.forgetChat(chatId)
      }
    }

    // The chats' TREE — folders, and nothing else.
    //
    // A full re-read rather than a patch, because the frame that triggers it names
    // only the folder that moved and carries no row: the list IS the answer, and
    // asking for it again is how two windows converge on one arrangement instead
    // of each keeping its own.
    //
    // The initial read is NOT here. The Chats panel takes it on mount
    // (use-agent-chat-folders.ts), and duplicating it would mean two GETs every
    // time the sidebar renders for the sake of a list that has not changed.
    //
    // Sequenced like the chat seed: two mutations in quick succession are two
    // reads in flight, and resolution order is not issue order — an older answer
    // landing last would put the folder that was just deleted back on screen.
    let folderSeq = 0
    const seedFolders = async () => {
      const seq = ++folderSeq
      try {
        const folders = await listChatFolders(wsId)
        if (cancelled || seq !== folderSeq) return
        getOrCreateWorkspaceStore(wsId).getState().seedAgentChatFolders(folders)
      } catch {
        /* non-fatal: the tree keeps the arrangement it has until the next frame */
      }
    }

    // THE PROVIDER LIST IS LOAD-BEARING, AND IT STARTS EMPTY.
    //
    // `INITIAL_AGENT_CHATS_STATE.providers` is `[]`, and every provider surface
    // reads emptiness as "there are none": Settings → Providers says "No
    // providers available.", the sidebar drops its New chat row, the New Tab
    // action and ⌘N do nothing. So a single lost fetch — a daemon restarting, a
    // dev hot-reload remount, one transient socket error — used to take the whole
    // agent UI down for the life of the workspace, silently, with nothing
    // retrying. That is the live report this exists to answer: healthy daemon,
    // both providers enabled on disk, empty UI.
    //
    // Three things make it recoverable, and they cover different failures:
    //   RETRY   — a one-off blip is retried immediately, bounded. No timer:
    //             a timer here would be a worse version of the next line.
    //   RECONNECT — the daemon coming back drops and re-opens the socket, and the
    //             {reconnected} sentinel re-drives this. That is the real backoff,
    //             driven by the thing that actually knows the daemon is answering.
    //   TOAST   — when every attempt fails we say so, because the alternative is
    //             a dead UI that explains nothing.
    let providerSeq = 0
    const seedProviders = async () => {
      const seq = ++providerSeq
      // The write generation this reseed is a snapshot of. A GET issued before a
      // preferences PUT and answered after it describes the server as it was
      // BEFORE the write, so publishing it would undo the user's change in both
      // copies — visibly, since the Providers rows are controlled off them. See
      // agent-providers-store's write generation.
      const writes = providerWriteGeneration()
      for (let attempt = 0; attempt < 3; attempt++) {
        try {
          const providers = await listProviders(wsId)
          if (cancelled) return
          // A newer read (a reconnect reseed) already owns the list.
          if (seq !== providerSeq) return
          // A preferences write landed while this was in flight; it is newer
          // truth and it already published to both copies.
          if (!isLatestProviderWrite(writes)) return
          getOrCreateWorkspaceStore(wsId).getState().setAgentProviders(providers)
          // Providers are machine-level, and the Settings dialog is global: give
          // the global store the same answer so opening Settings from anywhere
          // (Project Home, the projects screen) shows the real list instead of
          // claiming the daemon has none.
          useAgentProvidersStore.getState().setProviders(providers)
          providersUnreachableAnnounced = false
          return
        } catch {
          if (cancelled || seq !== providerSeq) return
        }
      }
      // ONCE PER OUTAGE, NOT ONCE PER WORKSPACE. This hook runs for every
      // MOUNTED workspace now (WorkspaceView, up to RETENTION_CAP = 6 at a
      // time), and the daemon being unreachable fails all of them at once —
      // for the same machine-level list, with the same sentence. Its only
      // previous mount point was a single sidebar panel, so the plain toast
      // was correct then and would stack six identical copies now.
      if (providersUnreachableAnnounced) return
      providersUnreachableAnnounced = true
      toast.error(
        'Could not load agent providers',
        'Crowbar could not reach the daemon. New chats are unavailable until it answers.',
      )
    }

    // The runner has arrived on `entered`: the pane following it now shows
    // `entered` (its row and group carry on), and any other pane that showed
    // `entered` goes. Runs even when no AgentChatPane is mounted.
    const followRunner = (runnerId: string, entered: string, closedProvider: string) => {
      const { panes, paneActions } = windowPaneStore.getState()
      const taker = Object.values(panes).find((p) => p.runnerId === runnerId)
      if (!taker) return
      const evicted = Object.values(panes).some((p) => p.chatId === entered && p.id !== taker.id)
      paneActions.retargetPane(taker.id, entered, runnerId)
      if (!evicted) return

      paneActions.setActivePane(taker.id)
      toast.info(
        'Conversation moved',
        `${closedProvider} was closed — that conversation is now in this pane.`,
      )
    }

    // A pane still following a runner the chat it shows no longer has lets go
    // of it at once — displacement asserts nothing about liveness, and an
    // `exited` may never come. It keeps its chat, and adopts a successor if one
    // arrives or renders dormant + Resume if none does.
    const releaseRunner = (runnerId: string, chatId: string) => {
      const { panes, paneActions } = windowPaneStore.getState()
      for (const pane of Object.values(panes)) {
        if (pane.runnerId === runnerId && pane.chatId === chatId) {
          paneActions.setPaneRunner(pane.id, null)
        }
      }
    }

    // A chat that starts working with no pane gets a background record — on a
    // LIVE edge only, once this mount's list has landed. A first sight through
    // the list (boot, remount, reconnect) is not an edge.
    const adoptIfViewless = (chatId: string) => {
      const { panes, paneActions } = windowPaneStore.getState()
      if (chatPaneIndex(panes).has(chatId)) return
      const projectId = getWorkspaceScope(wsId)?.projectId || resolveChatProjectId(chatId, wsId)
      if (projectId) paneActions.adoptBackgroundChat(chatId, projectId)
    }

    const applyLifecycle = (ev: AgentStreamEvent) => {
      const before = stateOf()
      const closedProvider = providerOn(before, ev.chatId)
      const outcome: ChatFrameOutcome = before.applyAgentChatFrame({
        chatId: ev.chatId,
        kind: ev.kind,
        runnerId: ev.runnerId,
        version: ev.version,
        chat: ev.chat ? mapChat(ev.chat) : undefined,
      })
      if (outcome.kind === 'deleted') {
        // Spec §9: a deleted chat's pane goes, and its view with it when that
        // was its last chat.
        windowPaneStore.getState().paneActions.forgetChat(ev.chatId)
        return
      }
      if (ev.kind === 'moved' && ev.runnerId && outcome.kind !== 'stale') {
        followRunner(ev.runnerId, ev.chatId, closedProvider)
      }
      const left = runnerLeft(outcome)
      if (left) releaseRunner(left, ev.chatId)
      if (startedWorking(outcome) && before.agentChats.listSeeded) adoptIfViewless(ev.chatId)
      if (ev.kind === 'turn_started' || ev.kind === 'turn_stopped') {
        // The thinking, the running tool's output and the plan belong to the
        // turn that just changed state. Deliberately NOT clearing
        // streamingMessages: an interrupted turn's CLI can keep producing and
        // complete on its own schedule; entries leave only once the ledger
        // holds them (useChatMessages).
        const st = stateOf()
        st.setAgentChatStreamingReasoning(ev.chatId, null)
        st.setAgentChatStreamingToolOutput(ev.chatId, null)
        st.setAgentChatStreamingPlan(ev.chatId, null)
      }
    }

    void seedChats()
    void seedProviders()

    const unsubscribe = wsManager.subscribe(`${chatBase(wsId)}/ws`, (frame) => {
      if (cancelled) return
      // Reconnect sentinel emitted by the manager after a socket drop+reopen —
      // reseed so pushes missed during the outage aren't lost.
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        // Invalidate transcript pages synchronously, before the repair GET: a
        // complete turn can be idle before and after the outage.
        stateOf().notifyAgentChatMessages()
        void seedChats()
        // Every folder frame dropped during the outage is a rearrangement this
        // client never heard about, and nothing else would ever ask again.
        void seedFolders()
        bumpTreeSignal()
        // The outage that dropped the socket is the same one that can have
        // emptied the providers, and this is the signal the daemon is back.
        void seedProviders()
        return
      }
      const ev = frame as AgentStreamEvent
      // The tree's own discriminator, read BEFORE the chatId guard: a folder
      // frame names no chat.
      if (ev.folderId) {
        void seedFolders()
        bumpTreeSignal()
        return
      }
      if (!ev.chatId) return
      // A chat row is a TREE row (design spec §3.1), so the sidebar has to hear
      // about a structural change exactly as it hears about a folder.
      if (!NON_STRUCTURAL_CHAT_KINDS.has(ev.kind)) bumpTreeSignal()
      const st = stateOf()
      // ANY other chat frame while this chat is marked compacting is proof the
      // compaction is over — a new turn, a delta, a snapshot. An idle snapshot
      // clears it too (the slice). No timer.
      if (
        ev.kind !== 'compaction_started' &&
        ev.kind !== 'compaction_stopped' &&
        st.agentChats.compacting[ev.chatId]
      ) {
        st.setAgentChatCompacting(ev.chatId, false)
      }
      switch (ev.kind) {
        case 'message_delta':
          if (!ev.message) return
          // A THOUGHT, not the answer: nothing in the ledger will ever match it,
          // so it must never reach streamingMessages.
          if (ev.message.kind === 'reasoning') {
            st.setAgentChatStreamingReasoning(ev.chatId, {
              id: ev.message.id,
              text: ev.message.text,
            })
            return
          }
          // A running tool's output — same contract as a thought.
          if (ev.message.kind === 'tool_output') {
            st.setAgentChatStreamingToolOutput(ev.chatId, {
              id: ev.message.id,
              text: ev.message.text,
            })
            return
          }
          streamingMessages.schedule(ev.chatId, ev.message)
          return
        case 'plan':
          st.setAgentChatStreamingPlan(ev.chatId, ev.plan ?? null)
          return
        case 'telemetry':
          st.setAgentChatTelemetry(ev.chatId, ev.telemetry ?? null)
          return
        case 'compaction_started':
          // The ledger's own record of a compaction is born resolved, so this
          // live push is the only place "in progress" is observable.
          st.setAgentChatCompacting(ev.chatId, true)
          return
        case 'compaction_stopped':
          st.setAgentChatCompacting(ev.chatId, false)
          return
        case 'prompt_settled':
          // Released one way or the other by whether the CLI provably took the
          // prompt: only then is the queued text spent and safe to discard.
          if (!ev.clientRequestId) return
          if (ev.promptConsumed) st.setAgentChatPromptSettled(ev.chatId, ev.clientRequestId)
          else st.setAgentChatPromptAbandoned(ev.chatId, ev.clientRequestId)
          return
        case 'worktree_state':
          return
        default:
          applyLifecycle(ev)
      }
    })

    return () => {
      cancelled = true
      unsubscribe()
      streamingMessages.dispose()
    }
  }, [wsId, scopeReady])
}

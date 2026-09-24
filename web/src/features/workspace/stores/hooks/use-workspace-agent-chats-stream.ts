import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import {
  chatBase,
  listChats,
  getChat,
  listProviders,
  listChatFolders,
  type AgentTelemetry,
  type AgentTerminalWait,
} from '@/features/agent/api/agent-api'
import {
  acceptChatRead,
  chatReadsApplied,
  claimChatRead,
  forgetChatRead,
  noteChatListRead,
} from '@/features/agent/lib/chat-read-order'
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
import type { PaneGroup } from '@/features/panes/types/pane'
import { toast } from '@/features/window/stores/toast-store'

type WorkspaceSnapshot = ReturnType<WorkspaceStore['getState']>

function panesNow(): PaneGroup[] {
  return Object.values(windowPaneStore.getState().panes)
}

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

// Where is this runner? Two independent answers, and we want the first that exists:
//
//   the CHAT LIST — the server's own placement, seeded and refetched. A chat is live
//     exactly while a runner sits on it, so the chat claiming `runnerId` IS the
//     runner→chat mapping, held for us, for every runner (not just the shown ones).
//   the PANE — where the client last saw it. The fallback matters on a cold client
//     whose chat list has not landed yet, or for a runner on a chat the list has
//     since replaced: a pane still following it is evidence.
function chatOfRunner(st: WorkspaceSnapshot, runnerId: string): string {
  const claimed = st.agentChats.chats.find((c) => c.liveRunnerId === runnerId)
  if (claimed) return claimed.id
  return panesNow().find((p) => p.runnerId === runnerId)?.chatId ?? ''
}

// The display name of the provider currently on `chatId`. Read BEFORE the chat is
// refetched — an arriving runner overwrites activeProviderId with its own, and the
// name of the CLI that just got closed is then gone for good.
function providerOn(st: WorkspaceSnapshot, chatId: string): string {
  const providerId = st.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? ''
  return st.agentChats.providers.find((p) => p.id === providerId)?.displayName ?? 'The agent'
}

// One wire frame on the agent feed (chatBase(wsId)/ws — repo-scoped, or the
// /home mount for a project-home workspace; see agent-api.ts's chatBase).
// THREE vocabularies ride it:
//
//   CHAT frames    — created / turn_started / turn_stopped / title_set / session_bound /
//                    placement_set / order_set / deleted / compaction_started /
//                    compaction_stopped. About the conversation. They name no process.
//   RUNNER frames  — started / session_bound / moved / displaced / exited. About the
//                    vendor-CLI PROCESS, which is a thing that moves between chats.
//   FOLDER frames  — folder_created / folder_updated / folder_deleted. About the tree the
//                    chats are arranged in, which is a second aggregate on its own route.
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
    | 'terminal_wait'
    | 'prompt_settled'
    | 'message_delta'
    | 'plan'
    | 'compaction_started'
    | 'compaction_stopped'
    | 'telemetry'
    | 'title_set'
    // A row MOVED in the tree — dragged into a folder, threaded under another
    // chat, or renumbered by the dense renumber a sibling's move triggered.
    // Both come off the chat aggregate's own commands (set_placement.go /
    // set_order.go); neither carries the new placement, for the same reason a
    // folder frame does not: the list is the answer, and it is refetched.
    | 'placement_set'
    | 'order_set'
    // The chat's own durable VENDOR moved — a provider switch, or a CLI moved
    // onto it by its own /clear. Like the two above it carries no new value and
    // is refetched: activeProviderId is derived, and the list is the answer.
    | 'provider_set'
    | 'deleted'
    | 'started'
    | 'moved'
    | 'displaced'
    | 'exited'
    | 'folder_created'
    | 'folder_updated'
    | 'folder_deleted'
  /** Set only on runner frames. Present ⟺ this frame is about a process. */
  runnerId?: string
  /**
   * Set only on folder frames. Present ⟺ this frame is about the tree.
   *
   * The same discriminator rule the runner half uses, and for the same reason: a
   * folder frame names no chat, so keying off `kind` alone would put it through a
   * switch that assumes one.
   *
   * The frame carries NO ROW — deliberately. This stream has no snapshot, and a
   * placement travelling on it would be a second source of truth that drifts from
   * the REST list the moment two windows write in the same breath. It says only
   * "the tree moved"; the answer is refetched.
   */
  folderId?: string
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
  /**
   * What the chat's CLI is blocked on that Crowbar CANNOT answer, on the
   * `terminal_wait` kind and nowhere else.
   *
   * Both edges ride this one kind, and the field's ABSENCE is the clearing edge:
   * present means "your agent is stuck behind a dialog", absent means "it isn't
   * any more". Carried on the frame rather than refetched for the same reason
   * `working` is — the user is looking at a pane that explains nothing, and an
   * answer a round trip later is an answer after they have given up.
   */
  terminalWait?: AgentTerminalWait
  /**
   * The client request id of one prompt that is over, on the `prompt_settled`
   * kind and nowhere else. See AgentChatsState.settledPrompts.
   */
  clientRequestId?: string
  /**
   * Whether anything actually proved the provider took that prompt, on the
   * `prompt_settled` kind. True is a built-in the CLI demonstrably ran (a
   * `/compact`); absent or false is the daemon's delivery timeout expiring with
   * no evidence of any kind.
   *
   * The queue item is the only place the user's typed text still exists at that
   * moment — the daemon's journal keeps a hash of it, never the text — so this
   * is what separates "safe to drop" from "the user's words would be destroyed".
   * Absent reads as false, which is the preserving answer.
   */
  promptConsumed?: boolean
  /**
   * An assistant message still being produced, on the `message_delta` kind.
   *
   * Carries the text SO FAR rather than the newest increment, so a client that
   * missed a frame is correct again on the next one and needs no reassembly of
   * its own. It is deliberately not in the ledger: a message still growing is a
   * view, and the ledger gets it once, when it is finished.
   */
  message?: {
    id: string
    text: string
    /**
     * WHICH stream this text belongs to. Absent is the agent's ANSWER — the
     * stream that existed before there was more than one, and the only one the
     * ledger ever records. `reasoning` is the agent thinking on the way there:
     * live-only, dropped at the turn edge, and rendered as a thought rather
     * than as the reply.
     */
    kind?: string
  }
  /**
   * The agent's own running to-do list, on the `plan` kind. Always the WHOLE
   * list — the newest one is the entire truth, so a client replaces rather than
   * merges and a missed frame costs nothing.
   */
  plan?: { text: string; status: string }[]
  /** The provider's newest usage report, on the `telemetry` kind — pushed as it
   *  lands so no client polls for it. */
  telemetry?: AgentTelemetry
}

/**
 * Subscribe to the agent WS for `wsId`'s chat scope while it is active. Seed via GET,
 * subscribe, reseed on the {reconnected} sentinel, and route each frame:
 *
 *  CHAT frames
 *   - turn_started / turn_stopped: the frame carries the server's folded `working`
 *     — write it through, no refetch. `turn_stopped` is NOT "idle": a chat waiting on
 *     a background subagent keeps spinning through it.
 *   - message_delta: an assistant message has grown. Transient; the ledger holds the
 *     finished message and this only makes the growing one visible.
 *   - prompt_settled: a delivered prompt is over without having produced a turn, so
 *     the composer's pending item can be released — nothing else will release it.
 *   - terminal_wait: the chat's CLI has become — or stopped being — blocked behind a
 *     prompt Crowbar cannot answer. The frame carries the whole answer; its absence
 *     on the payload is the clearing edge.
 *   - compaction_started / compaction_stopped: the chat is LIVE mid-compaction right
 *     now, or it just finished. The ledger's own interruption record for this is born
 *     already resolved (a bare /compact prompt never opens a tracked turn), so this
 *     push is the only place "in progress" is ever observable — see
 *     AgentChatsState.compacting. Self-healed on any OTHER chat frame and a bounded
 *     timeout, since compact_post is not reliable.
 *   - created: a new chat (and its ordering) may have appeared — reseed the whole list.
 *   - title_set / session_bound: refetch just that chat and upsert it.
 *   - deleted: drop the chat from the store and forget its pane (spec §9).
 *
 *  RUNNER frames — the pane is a viewport on a MOVING TARGET, and this is what moves it.
 *   - moved: the CLI switched conversation (the user typed /clear or /resume inside it).
 *     Retarget the pane that follows it, drop a pane that already held the destination,
 *     and invalidate BOTH chats.
 *   - displaced: Crowbar took the CLI off its chat. Let go of it at once.
 *   - started / session_bound / exited: refetch the chat named, and let the pane
 *     re-resolve (a chat nobody is on renders dormant + Resume).
 */
export function useWorkspaceAgentChatsStream(wsId: string): void {
  // `chatBase(wsId)` below (agent-api.ts) is `repoChatsBaseForWorkspace`,
  // which falls through to `workspaceBase` — and throws — the instant
  // project/repo scope is missing entirely, not just an owning chat id. This
  // hook runs for EVERY mounted workspace regardless of `active` (see its own
  // doc above: three surfaces need a hidden workspace's chats live), so it is
  // exactly the effect a force-mounted, never-navigated-to workspace hits
  // first — live-reported as an ErrorBoundary trip ("no project/repo scope
  // recorded for workspace …") right after a cold boot, whenever pane/Recents
  // state force-mounts a workspace before the sidebar's own repo fetch has
  // recorded its scope. Wait rather than crash; see useWorkspaceScopeReady's
  // own doc (workspace-scope.ts).
  const scopeReady = useWorkspaceScopeReady(wsId)
  useEffect(() => {
    if (!scopeReady) return
    let cancelled = false

    const stateOf = () => getOrCreateWorkspaceStore(wsId).getState()

    // Tells app-sync-provider.tsx's per-repo TREE subscription (Task 34: the
    // sidebar's folders resource has no dedicated push channel of its own any
    // more; Task D: neither do its chat rows) that THIS repo's tree may have
    // moved. Folders and chats ride ONE signal because they are one aggregate
    // and one tree — a folder IS a `domain.Chat` row (design spec §3.1) — and a
    // second near-identical store would only be two things to keep in step.
    //
    // SCOPED TO THIS WORKSPACE'S OWN REPO, and that is the cross-repo guard:
    // the id comes from the frame's own workspace scope, never from a broader
    // "something changed" broadcast, so a chat frame for repo A can never
    // reseed repo B. A workspace whose scope was never recorded bumps nothing
    // rather than guessing at a repo.
    const bumpTreeSignal = () => {
      const repoId = getWorkspaceScope(wsId)?.repoId
      if (repoId) useFolderSignalStore.getState().bump(repoId)
    }

    // READ ORDERING LIVES IN A MODULE (chat-read-order), NOT IN THIS CLOSURE.
    //
    // Every single-chat read that lands bumps a counter there; a list seed captures it
    // before it asks and refuses to publish a snapshot a fresher read has overtaken (see
    // seedChats), and each read carries a ticket so an earlier-issued one can never be
    // applied after a later-issued one (see refetchOne).
    //
    // It has to be shared because this hook is not the only thing that reads one chat and
    // writes it: agent-chat-pane's `adopt()` does it too, right after a resume, and that
    // write is the freshest fact in the app the moment it lands. A guard private to this
    // effect cannot see it. That is the live bug — adopt() attaches the runner the resume
    // just placed, then the `started` frame's refetch (issued FIRST, and answered from
    // before that placement) lands and blanks liveRunnerId, and the pane, its one revive
    // already spent, latches on "This agent has exited" over a CLI that is alive.

    // `message_delta` fires once per streamed token, each its own top-level WS
    // callback — outside anything React 18 batches. A fast provider can emit
    // several inside one animation frame; this collapses them into one store
    // write per chat per frame instead of one per token. See the batcher for why.
    const streamingMessages = createStreamingMessageBatcher((chatId, message) =>
      stateOf().setAgentChatStreamingMessage(chatId, message),
    )

    // Bounded self-heal for `compaction_started` with no matching
    // `compaction_stopped`. compact_post is NOT reliable (confirmed live:
    // most compactions on a small chat never produce one), so a design that
    // only clears `compacting` on that frame would leave the indicator stuck
    // showing forever whenever it doesn't arrive. COMPACTION_TIMEOUT_MS is
    // generous against the one real timing this session measured (~18s) —
    // this is a backstop, not the primary clearing path (that is any OTHER
    // lifecycle frame for the chat, below, since a new turn starting is
    // itself proof compaction is over).
    const COMPACTION_TIMEOUT_MS = 60_000
    const compactionTimers = new Map<string, ReturnType<typeof setTimeout>>()
    const clearCompactionTimer = (chatId: string) => {
      const timer = compactionTimers.get(chatId)
      if (timer === undefined) return
      clearTimeout(timer)
      compactionTimers.delete(chatId)
    }

    // ONLY THE MOST-RECENTLY ISSUED SEED MAY WRITE — the same guard `latestFetch`
    // carries in lib/store/loadable-slice.ts, and needed here for the same reason.
    // The applied-reads count protects a seed from being overtaken by a per-chat
    // READ; nothing protected it from being overtaken by ANOTHER SEED. Two ⌘N issue two
    // list reads, resolution order is not issue order, and a seed is a full REPLACE
    // — so an older snapshot landing last reinstates the list as it was before the
    // newer chat existed, and that chat disappears from the sidebar with nothing
    // scheduled to bring it back.
    let listSeq = 0
    // A reconnect may race a later live `created` reseed. Until one list read
    // actually lands, every seed must replace `working` from the server instead
    // of preserving the pre-outage map. Otherwise the newer created read wins
    // sequencing but also preserves the stale state reconnect was meant to repair.
    let needsReconnectReconcile = false
    // Whether a list read has landed since this effect mounted or the socket
    // last reconnected; until then the store's `working` map is not an answer.
    let listLanded = false
    // Chats first sighted through a live `created` frame after the list landed.
    const bornLive = new Set<string>()

    // The seed is a full RECONCILE, not a merge: it runs on first load AND on every
    // reconnect, and on reconnect it is the only thing that can repair frames the
    // socket dropped while it was down. seedAgentChats therefore drops chats the
    // server no longer has (a missed `deleted`) and replaces the working map from
    // each chat's server-folded value (repairing either missed turn edge).
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
    // `keepWorking` is threaded straight to seedAgentChats and is true for exactly one
    // caller: the `created` reseed. That reseed rides a LIVE socket (a new chat appeared,
    // the connection never dropped), so no turn frame was missed and clearing the working
    // map would needlessly blank the spinner on every OTHER mid-turn chat. Initial load and
    // reconnect leave it false — their list responses carry authoritative working state.
    const seedChats = async ({ keepWorking = false }: { keepWorking?: boolean } = {}) => {
      const seq = ++listSeq
      for (let attempt = 0; attempt < 3; attempt++) {
        try {
          const issuedAt = chatReadsApplied(wsId)
          const ticket = claimChatRead()
          const chats = await listChats(wsId)
          if (cancelled) return
          // A NEWER seed owns the list now: it asked later, so its answer is at
          // least as fresh as anything this one could ask for. Stand down entirely
          // (not `continue` — retrying would only race the newer seed again).
          if (seq !== listSeq) return
          if (chatReadsApplied(wsId) !== issuedAt) continue // overtaken in flight — old news

          const store = getOrCreateWorkspaceStore(wsId)
          const before = store.getState()
          before.hydrateAgentChatOrder()

          const present = new Set(chats.map((c) => c.id))
          const vanished: { id: string; owner: string }[] = []
          for (const c of before.agentChats.chats) {
            if (!present.has(c.id)) vanished.push({ id: c.id, owner: c.workspaceId })
          }

          if (keepWorking && !needsReconnectReconcile)
            store.getState().seedAgentChats(chats, { keepWorking: true })
          else store.getState().seedAgentChats(chats)
          needsReconnectReconcile = false
          listLanded = true
          for (const c of chats) if (c.working === true) adoptBornLive(c.id)
          // Every chat in this snapshot now carries an answer as fresh as `ticket`, so a
          // single-chat read ISSUED before this list request must no longer overwrite one.
          // The overtaken check above only ever asked the opposite question ("did a
          // per-chat read LAND while I was in flight"), which left a read issued before the
          // seed and landing after it free to walk straight over the reconcile.
          noteChatListRead(
            wsId,
            chats.map((c) => c.id),
            ticket,
          )

          // A chat missing from the list is only a SUSPECT: a repo-scoped list
          // can omit a project-home chat that still exists. Forget it only on
          // a definite not-found; the `deleted` frame forgets directly.
          for (const { id, owner } of vanished) void forgetIfGone(id, owner)
          return
        } catch {
          return /* seed failure is non-fatal — the WS stream still pushes */
        }
      }
    }

    // Only the chat's own workspace may confirm it gone: stores hold other
    // workspaces' chats, and asking through the wrong mount 404s a live chat.
    const forgetIfGone = async (chatId: string, recordOwner: string) => {
      const owner = recordOwner || resolveChatOwnerWorkspaceId(chatId)
      if (owner !== wsId) return
      try {
        await getChat(owner, chatId)
      } catch (err) {
        if (cancelled || !isNotFoundError(err)) return
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

    // Returns whether the chat actually landed in the store, so a caller that is about
    // to re-point a tab at it can decline when it did not: a tab pointed at a chat the
    // store has never heard of renders nothing at all.
    //
    // This is a POINT-IN-TIME read of ONE chat, and it outranks any list snapshot taken
    // before it — hence acceptChatRead, whose bump is what lets seedChats know it has been
    // overtaken (see there).
    //
    // It does NOT outrank a read of the same chat issued after it, and that is the whole
    // reason for the ticket. A spawn issues two of these back to back (`started`, then
    // `session_bound`) and a resume issues one here and one in the pane; the daemon can
    // answer the FIRST from before the runner placement it has already announced, so an
    // answer that arrives later can be older. Applied wholesale, it blanks liveRunnerId on
    // a chat whose CLI is alive, and nothing asks again.
    const refetchOne = async (chatId: string): Promise<boolean> => {
      const ticket = claimChatRead()
      try {
        const chat = await getChat(wsId, chatId)
        if (cancelled) return false
        // Overtaken: a read issued LATER already applied, so the store holds a row fresher
        // than this one and this answer is a snapshot of the past. Drop it — but answer the
        // caller's actual question (is the chat in the store?) from the STORE, not from a
        // payload we have just declared unfit to write.
        if (!acceptChatRead(wsId, chatId, ticket)) {
          return getOrCreateWorkspaceStore(wsId)
            .getState()
            .agentChats.chats.some((c) => c.id === chatId)
        }
        getOrCreateWorkspaceStore(wsId).getState().upsertAgentChat(chat, ticket)
        return true
      } catch {
        /* a not-found here is handled by the deleted frame path */
        return false
      }
    }

    // The runner has arrived on `entered`: the pane following it now shows
    // `entered` (its row and group carry on), and any other pane that showed
    // `entered` goes (Law 4). Runs even when no AgentChatPane is mounted.
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

    // A chat that starts working with no pane gets a background record — only on
    // a LIVE edge: after this mount's list read landed, the store held the chat
    // idle, or the chat was born live on this stream (its first working sight,
    // from a frame or its own reseed, adopts). A first sight through a snapshot
    // (boot, remount, reconnect) is not an edge.
    const heldIdle = (st: WorkspaceSnapshot, chatId: string) =>
      listLanded &&
      st.agentChats.working[chatId] !== true &&
      st.agentChats.chats.some((c) => c.id === chatId)

    const adoptBornLive = (chatId: string) => {
      if (!bornLive.delete(chatId)) return
      adoptIfViewless(chatId)
    }

    const adoptIfViewless = (chatId: string) => {
      const { panes, paneActions } = windowPaneStore.getState()
      if (chatPaneIndex(panes).has(chatId)) return
      const projectId = getWorkspaceScope(wsId)?.projectId || resolveChatProjectId(chatId, wsId)
      if (projectId) paneActions.adoptBackgroundChat(chatId, projectId)
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
          // no pane following it and no chat claiming it, and does nothing.
          const held = chatOfRunner(st, ev.runnerId)
          const { paneActions } = windowPaneStore.getState()
          for (const pane of panesNow()) {
            // The pane keeps its chat and lets the runner go; it adopts a successor
            // if one arrives, and renders dormant + Resume if none does.
            if (pane.runnerId === ev.runnerId) paneActions.setPaneRunner(pane.id, null)
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

    const unsubscribe = wsManager.subscribe(`${chatBase(wsId)}/ws`, (frame) => {
      if (cancelled) return
      // Reconnect sentinel emitted by the manager after a socket drop+reopen —
      // reseed so pushes missed during the outage aren't lost.
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        // Invalidate transcript pages synchronously, before the repair GET. A
        // complete turn can be idle before and after the outage, and a failed or
        // superseded list request must not make its messages invisible forever.
        stateOf().notifyAgentChatMessages()
        needsReconnectReconcile = true
        listLanded = false
        bornLive.clear()
        void seedChats()
        // Folders too, and for exactly the reason the chat list is reseeded here:
        // every folder frame dropped during the outage is a rearrangement this
        // client never heard about, and nothing else would ever ask again.
        void seedFolders()
        // ...and the SIDEBAR's own tree pipeline (folders AND chat rows), which
        // watches this same signal rather than this hook's own workspace-store
        // state. Every frame dropped during the outage is a rearrangement this
        // client never heard about, chat rows included.
        bumpTreeSignal()
        // Providers too: the outage that dropped the socket is the same one that
        // can have emptied them, and this is the app's own signal that the daemon
        // is answering again. Without it a workspace that lost its providers
        // stayed dead until the user reopened it.
        void seedProviders()
        return
      }
      const ev = frame as AgentStreamEvent
      // The discriminator, and the first branch for that reason — see AgentStreamEvent.
      if (ev.runnerId) {
        onRunnerFrame({ ...ev, runnerId: ev.runnerId })
        return
      }
      // The tree's own discriminator. It has to be read BEFORE the chatId guard
      // below: a folder frame names no chat, so that guard would drop every one of
      // them — which is precisely how folders came to not sync across windows.
      //
      // All three kinds do the same thing, because all three mean the same thing
      // here: the arrangement is not what this client thinks it is.
      if (ev.folderId) {
        void seedFolders()
        bumpTreeSignal()
        return
      }
      if (!ev.chatId) return
      // A chat row is a TREE row (design spec §3.1), so the sidebar has to hear
      // about it exactly as it hears about a folder. Read before the switch
      // below rather than repeated inside four of its branches: the question
      // "did this move the tree?" is about the frame's kind alone, and the
      // branches below are about what the WORKSPACE STORE does with it, which
      // is a different question with a different answer per kind.
      if (!NON_STRUCTURAL_CHAT_KINDS.has(ev.kind)) bumpTreeSignal()
      const st = stateOf()
      // Self-heal: ANY other chat frame arriving while this chat is marked
      // compacting is itself proof the compaction is no longer the live
      // state — a new turn, a message delta, a terminal-wait edge, none of
      // those can happen mid-compaction. This is the PRIMARY way a missing
      // compact_post gets noticed quickly; the timeout below is only the
      // absolute backstop for a chat that goes silent altogether.
      if (
        ev.kind !== 'compaction_started' &&
        ev.kind !== 'compaction_stopped' &&
        st.agentChats.compacting[ev.chatId]
      ) {
        clearCompactionTimer(ev.chatId)
        st.setAgentChatCompacting(ev.chatId, false)
      }
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
          {
            const wasIdle = heldIdle(st, ev.chatId)
            st.setAgentChatWorking(ev.chatId, ev.working === true)
            if (ev.working === true && listLanded && bornLive.has(ev.chatId))
              adoptBornLive(ev.chatId)
            else if (wasIdle && ev.working === true) adoptIfViewless(ev.chatId)
          }
          // The thinking belonged to the turn that just changed state, and the
          // answer supersedes it. Unlike streamingMessages below there is nothing
          // to preserve across the edge: a thought is never recorded, so a stale
          // one can only mislead. The server drops its own buffer on the same
          // edge (turn/reasoning.go).
          st.setAgentChatStreamingReasoning(ev.chatId, null)
          st.setAgentChatStreamingToolOutput(ev.chatId, null)
          st.setAgentChatStreamingPlan(ev.chatId, null)
          //
          // Deliberately NOT clearing streamingMessages[chatId] here (tried,
          // reverted): "interrupted" does not mean dead. Stopping a turn is a
          // graceful request, not a kill — the CLI it was asked to stop can
          // keep producing output and complete its OWN turn on its own
          // schedule, arriving under its own message id well after a
          // DIFFERENT turn (a provider switch mid-interrupt) has already
          // started. A blanket clear on the next turn_started throws that
          // still-alive entry away — the reader watches it vanish mid-
          // sentence. Entries are removed only by useChatMessages's own
          // dedup-against-the-ledger check, same as any other item.
          return
        case 'message_delta':
          if (!ev.message) return
          // A THOUGHT, not the answer. It must never reach streamingMessages:
          // nothing in the ledger will ever match it, so useChatMessages' own
          // prune-against-the-ledger pass could not retire it and it would sit in
          // the transcript as an assistant bubble forever. It is also the frame
          // that fills the long silence while a reasoning model works, which is
          // the whole reason it is carried at all.
          if (ev.message.kind === 'reasoning') {
            st.setAgentChatStreamingReasoning(ev.chatId, {
              id: ev.message.id,
              text: ev.message.text,
            })
            return
          }
          // A running tool's output. Same contract as a thought: nothing in the
          // ledger will ever match it (the tool's full output arrives once, on
          // the completed call), so it must never reach streamingMessages either.
          if (ev.message.kind === 'tool_output') {
            st.setAgentChatStreamingToolOutput(ev.chatId, {
              id: ev.message.id,
              text: ev.message.text,
            })
            return
          }
          // The agent is mid-sentence. This is the only frame in the feed that is
          // not a record of anything — it is replaced by the ledger's own copy the
          // moment the message completes. Batched to the next frame rather than
          // written straight through — see streamingMessages above.
          streamingMessages.schedule(ev.chatId, ev.message)
          return
        case 'plan':
          // Wholesale replace: see the frame's own doc above.
          st.setAgentChatStreamingPlan(ev.chatId, ev.plan ?? null)
          return
        case 'telemetry':
          st.setAgentChatTelemetry(ev.chatId, ev.telemetry ?? null)
          return
        case 'compaction_started':
          // The ledger's own interruption record for this is born already
          // resolved (see AgentChatsState.compacting's doc comment) — this
          // live push is the only place "in progress" is ever observable.
          clearCompactionTimer(ev.chatId)
          st.setAgentChatCompacting(ev.chatId, true)
          compactionTimers.set(
            ev.chatId,
            setTimeout(() => {
              compactionTimers.delete(ev.chatId)
              if (cancelled) return
              stateOf().setAgentChatCompacting(ev.chatId, false)
            }, COMPACTION_TIMEOUT_MS),
          )
          return
        case 'compaction_stopped':
          clearCompactionTimer(ev.chatId)
          st.setAgentChatCompacting(ev.chatId, false)
          return
        case 'prompt_settled':
          // A prompt Crowbar delivered turned out not to produce a turn. The
          // composer's pending queue is waiting on a user message that is never
          // coming, and this frame is the only thing that releases it.
          //
          // WHICH way it is released is the difference between a tidy composer
          // and losing the user's work. `promptConsumed` says the CLI actually
          // ran it — a built-in like `/compact`, which announces nothing by
          // design — and only then is the queued text spent and safe to discard.
          // Without that proof the daemon is merely reporting that its delivery
          // timeout expired, and the queue item is the last copy of what the user
          // typed, so it is kept and surfaced as failed instead.
          if (!ev.clientRequestId) return
          if (ev.promptConsumed) st.setAgentChatPromptSettled(ev.chatId, ev.clientRequestId)
          else st.setAgentChatPromptAbandoned(ev.chatId, ev.clientRequestId)
          return
        case 'terminal_wait':
          // The frame IS the answer, both ways round: a present payload raises
          // the "waiting in the terminal" state, an absent one clears it. The
          // daemon publishes only on a CHANGE, so a chat parked for an hour has
          // sent exactly one of these.
          st.setAgentChatTerminalWait(ev.chatId, ev.terminalWait ?? null)
          return
        case 'deleted': {
          st.removeAgentChat(ev.chatId)
          forgetChatRead(wsId, ev.chatId)
          // Spec §9: a deleted chat's pane goes, and its view with it when
          // that was its last chat.
          windowPaneStore.getState().paneActions.forgetChat(ev.chatId)
          return
        }
        case 'created':
          if (listLanded && !st.agentChats.chats.some((c) => c.id === ev.chatId))
            bornLive.add(ev.chatId)
          // New chat + ordering — reseed the whole list, but KEEP working: the socket is
          // live, so no other chat's turn state was missed and clearing it would blank
          // every other mid-turn chat's spinner.
          void seedChats({ keepWorking: true })
          return
        default:
          void refetchOne(ev.chatId) // title_set / session_bound
      }
    })

    return () => {
      cancelled = true
      unsubscribe()
      streamingMessages.dispose()
      for (const timer of compactionTimers.values()) clearTimeout(timer)
      compactionTimers.clear()
    }
  }, [wsId, scopeReady])
}

import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  AgentChat,
  AgentChatFolder,
  AgentProvider,
  AgentTerminalWait,
} from '@/features/agent/api/agent-api'
import { clearPersistedPromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
import { chatReadMark } from '@/features/agent/lib/chat-read-order'
import type { TranscriptScrollPosition } from '@/features/agent/hooks/use-transcript-anchor'

// The queue that reads these is itself capped, so an id older than this window is
// already unreachable by anything that could act on it.
const SETTLED_PROMPTS_PER_CHAT = 20

const orderKey = (wsId: string) => `crowbar:agent-chat-order:${wsId}`

function loadOrder(wsId: string): string[] {
  try {
    const raw = localStorage.getItem(orderKey(wsId))
    return raw ? (JSON.parse(raw) as string[]) : []
  } catch {
    return []
  }
}

function saveOrder(wsId: string, order: string[]): void {
  try {
    localStorage.setItem(orderKey(wsId), JSON.stringify(order))
  } catch {
    /* quota / private mode — best effort */
  }
}

/** Order chats by the client-persisted order first (chats named in `order`, in
 *  that sequence), then append any chat absent from `order` sorted by createdAt
 *  DESCENDING — newest first. Pure/testable.
 *
 *  Newest-first is the default because a chat you just started is the one you
 *  want, and because the New Tab's "Recent" list takes its top-N straight from
 *  this function (new-tab-view.tsx): appending newest LAST buried the newest chat
 *  and, once past the list cap, hid it entirely. Chats the user has explicitly
 *  dragged still lead, in the sequence they chose — an explicit arrangement
 *  outranks recency. */
export function orderedChats(chats: AgentChat[], order: string[]): AgentChat[] {
  const byId = new Map(chats.map((c) => [c.id, c]))
  const pinned = order.map((id) => byId.get(id)).filter((c): c is AgentChat => c !== undefined)
  const pinnedIds = new Set(pinned.map((c) => c.id))
  const rest = chats
    .filter((c) => !pinnedIds.has(c.id))
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  return [...pinned, ...rest]
}

/** The enabled providers, in the backend's priority order — the single subset
 *  every "New chat" surface offers (the first is the provider a unified New-chat
 *  action opens). A disabled provider is hidden entirely (spec §2.2), so it never
 *  appears here. Pure selector; use with a narrow `useStore(store, …)` read. */
export const selectEnabledProviders = (s: WorkspaceState): AgentProvider[] =>
  s.agentChats.providers.filter((p) => p.enabled)

export interface AgentChatsState {
  chats: AgentChat[]
  /**
   * Is this chat's agent busy — the spinner map, keyed by chat id.
   *
   * NOT derived here. Every value is the server's own folded answer
   * (domain.AgentChat.Working), carried on the lifecycle frame that announced the
   * change and written through verbatim.
   *
   * That is deliberate and load-bearing. This map used to be re-derived from the
   * frame KIND — `turn_stopped` → idle — which stopped being true the moment a CLI
   * could hand work to a background subagent and go quiet waiting for it: claude
   * ends its turn right there, so the row went dark under an agent that was still
   * working. Re-deriving it here at all means a second copy of a rule that already
   * exists in Go, and two copies can disagree. One fold, on the server; this map
   * just displays it.
   */
  working: Record<string, boolean>
  /**
   * Which chats have a CLI blocked behind a prompt Crowbar CANNOT answer, keyed
   * by chat id. PRESENCE is the verdict: an absent entry means nothing is
   * blocking that chat, and an entry with `kind: ''` means the daemon knows only
   * that something is.
   *
   * A map rather than a field on the chat row, and for the same reason `working`
   * is one: it arrives on its own lifecycle frame and has to be writable without
   * a round trip, so it must not require replacing a chat object nobody else
   * asked to change. It also keeps the read a PRIMITIVE — a consumer selects
   * `terminalWaits[id]?.kind`, which is a string or undefined, so a narrow
   * selector cannot churn on object identity.
   *
   * Never derived here. The gates that decide it (a live runner, an idle chat, no
   * prompt the chat could answer, and a screen matching a declared needle) are
   * folded once in the daemon against its own terminal model; a second copy of
   * that rule in TypeScript would be a second thing to get wrong.
   */
  terminalWaits: Record<string, AgentTerminalWait>
  /**
   * Which chats are LIVE mid-compaction right now, keyed by chat id. PRESENCE
   * is the verdict, same convention as terminalWaits.
   *
   * Driven ENTIRELY by the `compaction_started`/`compaction_stopped` WS
   * frames (use-workspace-agent-chats-stream.ts) — never by the ledger's own
   * interruption record. That record is born already resolved (a bare
   * /compact prompt never opens a tracked turn), so it can never answer "is
   * one happening right now"; only the live push can. The stream also
   * self-heals this on any other lifecycle frame and a bounded timeout,
   * because compact_post is not reliable — see the stream's own comments.
   *
   * A THIRD backstop lives in seedAgentChats: both of the above require the
   * app to still be running to fire, so a compaction_stopped lost for good
   * (the app closed for the whole compaction window) leaves this stuck true
   * forever without it. seedAgentChats clears — never sets — an entry once
   * its own fresh `working` comes back false, since a chat cannot genuinely
   * be mid-compaction and idle at once.
   */
  compacting: Record<string, boolean>
  /**
   * Client request ids the daemon has reported as SETTLED: delivered to a CLI,
   * and over, without ever having produced a turn.
   *
   * The composer's pending queue normally resolves an item when the prompt it sent
   * turns up in the ledger as a user message. A provider's own built-in command
   * never produces one — the CLI handles it and announces nothing — so the item
   * would wait forever on evidence that is not coming. This is the other way an
   * item can be resolved.
   *
   * Bounded to the most recent SETTLED_PROMPTS_PER_CHAT per chat. An id only
   * matters while the queue still holds it, and the queue is itself capped, so
   * anything older than that window is already unreachable.
   */
  settledPrompts: Record<string, string[]>
  /**
   * The assistant message(s) each chat is CURRENTLY producing, if any.
   *
   * An ARRAY, not a single slot: a turn can have more than one message item
   * open (Codex splits a reply across several `agentMessage` items with
   * distinct ids; Claude's turns only ever have one, so this is always
   * length-0-or-1 for it). Upserted by id, never overwritten wholesale — a
   * new item starting must not silently drop an earlier one still streaming.
   *
   * Transient by design and never persisted: entries are dropped ONLY once
   * the message lands in the ledger (compared by text — see useChatMessages),
   * never on a new turn starting — "interrupted" is a graceful request, not
   * a kill, so the stopped turn's CLI can keep producing and complete on its
   * own schedule after a different turn has already begun. Keeping it out
   * of the ledger is what stops roughly 1.4 durable writes a second per
   * streaming chat to store text that is superseded a moment later.
   */
  streamingMessages: Record<string, { id: string; text: string }[]>
  /** Monotonic notification counter. It advances for every server turn state
   *  write even when React batches a fast true→false pair into one render, and
   *  on an authoritative reconnect reseed because a complete idle→idle turn
   *  may have occurred while the socket was down. */
  turnRevision: Record<string, number>
  /**
   * Where the reader last left each chat's transcript, keyed by chat id —
   * read once when a chat's AgentTranscript (re)mounts (a chat remounts
   * wholesale on every switch, `key={wsId:chatId}` in AgentChatPane, so
   * nothing else survives a switch-away/switch-back to carry this).
   *
   * In-memory only, deliberately never persisted to disk (unlike
   * agent-chat-order's localStorage above): "still hot" means this running
   * session, not "restore across an app restart" — a cold app open has
   * nothing more useful to land on than the newest message anyway.
   */
  scrollPositions: Record<string, TranscriptScrollPosition>
  order: string[]
  activeChatId: string | null
  providers: AgentProvider[]
  /**
   * The workspace's chat FOLDERS, straight from the daemon.
   *
   * Beside the chats rather than inside them because a folder is a peer of a
   * chat: the two interleave at every level of the tree and sort on one shared
   * `order`. Nothing here is derived — the tree is built from these two arrays by
   * a pure function the panel memoises.
   */
  folders: AgentChatFolder[]
}

export interface AgentChatsSlice {
  agentChats: AgentChatsState
  seedAgentChats: (chats: AgentChat[], opts?: { keepWorking?: boolean }) => void
  /** Invalidate every mounted chat transcript after a socket outage. This is
   * independent of the reconnect GET so even a failed/superseded read cannot
   * hide a complete idle-to-idle turn that happened while disconnected. */
  notifyAgentChatMessages: () => void
  /** Upsert one chat from a single-chat read. `readTicket` is that read's
   *  chat-read-order ticket; pass it whenever there is one, so the cross-chat runner
   *  eviction can tell a stale claim from a current one (see the implementation). */
  upsertAgentChat: (chat: AgentChat, readTicket?: number) => void
  removeAgentChat: (chatId: string) => void
  /** Write the server's folded busy state for a chat. Never computed client-side. */
  setAgentChatWorking: (chatId: string, working: boolean) => void
  /**
   * Write — or clear, with null — the daemon's answer to "is this chat's CLI
   * blocked on something only the terminal can clear?".
   *
   * Both edges travel, because the CLEARING edge is what takes the banner down
   * when somebody answers the dialog at the terminal or the CLI dies behind it.
   */
  setAgentChatTerminalWait: (chatId: string, wait: AgentTerminalWait | null) => void
  /** Write — or clear, with false — whether a chat is LIVE mid-compaction
   *  right now. See AgentChatsState.compacting for why this must never be
   *  derived from the ledger. */
  setAgentChatCompacting: (chatId: string, active: boolean) => void
  /** Record that one delivered prompt is over without having produced a turn. */
  setAgentChatPromptSettled: (chatId: string, clientRequestId: string) => void
  /** Upsert (by id) one message a chat is mid-way through saying, or clear
   *  ALL of a chat's in-flight messages with null (a new turn starting). */
  setAgentChatStreamingMessage: (
    chatId: string,
    message: { id: string; text: string } | null,
  ) => void
  /** Drop the given ids' entries once the ledger has recorded them for real —
   *  see useChatMessages' streamingBubbles for the matching id computation
   *  this is the store-side twin of. NOT a blanket clear on a turn boundary:
   *  that was tried and reverted (use-workspace-agent-chats-stream.ts),
   *  because an interrupted runner's stream can legitimately keep growing
   *  after a new turn starts, and wiping the array on turn_stopped/started
   *  deleted a reply that was still arriving. Pruning by CONFIRMED id instead
   *  is what keeps the array bounded without that regression: an entry is
   *  only ever removed once its own content is durably persisted elsewhere. */
  pruneAgentChatStreamingMessages: (chatId: string, ids: string[]) => void
  /** Record wherever the reader left a chat's transcript, for its next
   *  mount this session to restore — see AgentChatsState.scrollPositions. */
  setAgentChatScrollPosition: (chatId: string, position: TranscriptScrollPosition) => void
  /**
   * Write the chat's sticky model / effort selection after the server ACCEPTED it.
   *
   * The selection endpoint answers 202 with no body and rides no lifecycle frame,
   * so nothing else would bring the accepted pair back into the store — the picker
   * would keep painting from a value the server has already moved past until the
   * next full read. '' on either half means the provider's own default.
   */
  setAgentChatSelection: (chatId: string, model: string, effort: string) => void
  setAgentChatOrder: (order: string[]) => void
  hydrateAgentChatOrder: () => void
  setActiveAgentChatId: (chatId: string | null) => void
  setAgentProviders: (providers: AgentProvider[]) => void
  /** Replace the folder list from an authoritative GET. */
  seedAgentChatFolders: (folders: AgentChatFolder[]) => void
  /**
   * Write folders that a server round trip returned — the row that was created or
   * renamed, and every sibling its dense renumber displaced.
   *
   * One action for both halves because they are one answer: applying the row and
   * dropping the `shifted` beside it is how a level ends up holding two rows that
   * both think they are third.
   */
  applyAgentChatFolders: (folders: readonly AgentChatFolder[]) => void
  /**
   * Delete a folder, promoting its children to the folder's own parent.
   *
   * A folder holds no conversation, so the chats outlive it. Deliberately UNLIKE
   * a chat delete, which takes its threads: a thread exists to continue its
   * parent, and outliving it would strand it reading a context that is gone.
   */
  removeAgentChatFolder: (folderId: string) => void
  /** Move a chat in the tree. Optimistic; the daemon's answer overwrites it. */
  setAgentChatPlacement: (chatId: string, parentId: string, order: number) => void
}

export const INITIAL_AGENT_CHATS_STATE: AgentChatsState = {
  chats: [],
  working: {},
  terminalWaits: {},
  compacting: {},
  settledPrompts: {},
  streamingMessages: {},
  turnRevision: {},
  scrollPositions: {},
  order: [],
  activeChatId: null,
  providers: [],
  folders: [],
}

export const createAgentChatsSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  AgentChatsSlice
> = (set, get) => ({
  agentChats: { ...INITIAL_AGENT_CHATS_STATE },

  // Reconcile the chat list against an AUTHORITATIVE GET — the initial load and
  // every WS-reconnect reseed. Unlike a loop of upserts, this is a full
  // replacement, because the reseed's whole job is to repair what the socket
  // missed while it was down:
  //
  //  - Chats absent from the response are DROPPED. A `deleted` frame lost during
  //    the outage would otherwise leave a ghost row that never goes away.
  //  - The working map is REPLACED from each chat's server-folded `working` value.
  //    This both clears a spinner whose `turn_stopped` was lost during the outage
  //    and restores a spinner for a turn already in flight when the client joins.
  //    Older daemons omit the field; omission still grounds to idle.
  //
  // `keepWorking` is the one exception, and it exists for the `created` reseed. That
  // reseed rides a LIVE socket — it fires because a new chat appeared, not because the
  // connection dropped — so NO turn frame was missed and every surviving chat's working
  // state is still the truth. Clearing it there is a bug: it blanks the spinner on every
  // OTHER mid-turn chat the instant a new chat opens, until each runs another turn. So
  // when told working is known, keep it, and only forget entries for chats that are gone.
  seedAgentChats: (chats, opts) => {
    const prev = get().agentChats
    const present = new Set(chats.map((c) => c.id))
    const vanished = prev.chats.reduce<string[]>((ids, chat) => {
      if (!present.has(chat.id)) ids.push(chat.id)
      return ids
    }, [])
    const pruned = prev.order.filter((id) => present.has(id))

    // Chats that appeared since the last seed. A `created` frame reseeds the whole
    // list, so this is where a brand-new chat first shows up. Only meaningful once
    // a previous list EXISTS: on the first seed of a session every chat looks new,
    // and promoting them all would scramble the very arrangement being restored.
    // Both membership tests are Sets — this runs on every reseed, over the whole
    // list, so a nested scan here would be quadratic in the chat count.
    const knownIds = new Set(prev.chats.map((c) => c.id))
    const prunedIds = new Set(pruned)
    const arrived =
      prev.chats.length === 0
        ? []
        : chats
            .filter((c) => !knownIds.has(c.id) && !prunedIds.has(c.id))
            .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
            .map((c) => c.id)

    // A new chat joins the TOP of an existing arrangement. One drag pins the whole
    // list, so without this every later chat sinks below every pinned one — and off
    // the New Tab's capped "Recent" list entirely. With NO saved order the
    // newest-first sort in orderedChats already puts it first, and writing one here
    // would start pinning a list the user never arranged.
    const nextOrder = pruned.length > 0 && arrived.length > 0 ? [...arrived, ...pruned] : pruned

    set((s) => {
      s.agentChats.chats = chats
      if (opts?.keepWorking) {
        for (const id of Object.keys(s.agentChats.working)) {
          if (!present.has(id)) delete s.agentChats.working[id]
        }
        // A live `created` reseed preserves surviving frame-derived answers, but
        // newly arrived chats have no map entry yet. Seed only a positive server
        // answer for those rows; absent/false is already represented by omission.
        for (const chat of chats) {
          if (s.agentChats.working[chat.id] === undefined && chat.working === true) {
            s.agentChats.working[chat.id] = true
          }
        }
      } else {
        s.agentChats.working = {}
        for (const chat of chats) {
          if (chat.working === true) s.agentChats.working[chat.id] = true
        }
      }
      // The blocked-in-the-terminal map, reconciled on exactly the same terms as
      // `working` above and for the same reason: both are server-folded facts the
      // list response carries, and both have a lifecycle frame that can be lost
      // while the socket is down. A live `created` reseed (keepWorking) leaves the
      // surviving answers alone — no frame was missed — and only seeds chats it
      // has never seen; an authoritative reconnect replaces the lot.
      if (opts?.keepWorking) {
        for (const id of Object.keys(s.agentChats.terminalWaits)) {
          if (!present.has(id)) delete s.agentChats.terminalWaits[id]
        }
        for (const chat of chats) {
          if (s.agentChats.terminalWaits[chat.id] === undefined && chat.terminalWait) {
            s.agentChats.terminalWaits[chat.id] = chat.terminalWait
          }
        }
      } else {
        s.agentChats.terminalWaits = {}
        for (const chat of chats) {
          if (chat.terminalWait) s.agentChats.terminalWaits[chat.id] = chat.terminalWait
        }
      }
      // `compacting` has no ledger record to fall back on (see
      // AgentChatsState.compacting) and, unlike `working`/`terminalWaits` above,
      // cannot be REBUILT from this response — there is no `chat.compacting`
      // field, only the live push knows "in progress". What this GET's own
      // `working` CAN do is prove one stale: compaction keeps a chat busy the
      // same as any other turn, so a fresh read showing it idle is the same
      // proof the live self-heal already trusts for any other frame — clear
      // only, never set. This is what repairs a `compaction_stopped` lost for
      // good (the app closed for the whole compaction window, missing both the
      // frame and the local backstop timer that died with it).
      for (const chat of chats) {
        if (s.agentChats.compacting[chat.id] && chat.working !== true) {
          delete s.agentChats.compacting[chat.id]
        }
      }
      for (const id of Object.keys(s.agentChats.turnRevision)) {
        if (!present.has(id)) delete s.agentChats.turnRevision[id]
      }
      s.agentChats.order = nextOrder
      if (s.agentChats.activeChatId !== null && !present.has(s.agentChats.activeChatId)) {
        s.agentChats.activeChatId = null
      }
    })

    if (arrived.length > 0 && nextOrder !== pruned) saveOrder(get().workspaceId, nextOrder)
    for (const chatId of vanished) clearPersistedPromptQueue(get().workspaceId, chatId)
  },

  notifyAgentChatMessages: () => {
    set((s) => {
      for (const chat of s.agentChats.chats) {
        s.agentChats.turnRevision[chat.id] = (s.agentChats.turnRevision[chat.id] ?? 0) + 1
      }
    })
  },

  // Upsert ONE chat, refetched because a WS frame said it changed.
  //
  // A runner is placed on exactly ONE chat — the backend enforces it ("of everyone
  // here, the newest arrival stays and the rest go"), and this projection has to hold
  // the same invariant, because it is updated one chat at a time.
  //
  // The case that forces it: a runner MOVES (the user typed /clear inside the CLI).
  // The `moved` frame names the chat it moved INTO, so only that chat is refetched —
  // and the chat it LEFT keeps a liveRunnerId that is now a lie. Two chats would claim
  // one runner, and a pane following that runner would resolve to whichever came first
  // in the array (the stale one) and never follow. So an arriving chat evicts its
  // runner from wherever else it was: the fresh, server-sourced fact wins.
  //
  // This can only ever CLEAR a claim that a newer backend read contradicts — it never
  // invents liveness. terminalSessionId goes with it: a chat with no runner has no PTY
  // to attach, and leaving one behind would let a pane attach a dead session.
  //
  // `readTicket` is the chat-read-order ticket of the READ this row came from, and it is
  // what keeps the eviction below from re-opening the bug the registry closes. Eviction
  // is the one write here that touches a chat OTHER than the one handed in, so
  // `acceptChatRead` — which asks only about the row being written — cannot cover it.
  // The reachable sequence: runner R spawns onto A (`started` refetches A at ticket T1,
  // in flight); R then moves to B before T1 lands. At that moment NO chat claims R and no
  // pane follows it, so `chatOfRunner` resolves nothing and A is never refetched — A's
  // mark stays at the seed's T0. B is read at T2 and correctly claims R. T1 then lands,
  // is legitimately accepted for A (T1 > T0), and its eviction blanked R off B without
  // ever consulting B — the same "This agent has exited" over a live CLI, through a door
  // the read ordering never saw.
  //
  // Resolved by asking WHICH ANSWER ABOUT THIS RUNNER IS NEWER. If some other chat holds
  // it under a mark newer than the ticket driving this write, that chat is the newer fact
  // and THIS claim is the stale one: the row still lands (its title, ordering, provider
  // are all fine), but it does not take a runner it has already lost, and it evicts
  // nobody. The one-runner-one-chat invariant holds either way — which is why this is
  // dropping the claim rather than simply skipping the eviction, since skipping would
  // leave two chats claiming R and a pane resolving to whichever came first in the array.
  //
  // Omitting the ticket keeps the pre-ordering behaviour exactly, for callers with no
  // read behind them.
  upsertAgentChat: (chat, readTicket) =>
    set((s) => {
      const idx = s.agentChats.chats.findIndex((c) => c.id === chat.id)
      if (idx === -1) s.agentChats.chats.push(chat)
      else s.agentChats.chats[idx] = chat

      if (chat.liveRunnerId && readTicket !== undefined) {
        const wsId = get().workspaceId
        const newerClaimant = s.agentChats.chats.some(
          (c) =>
            c.id !== chat.id &&
            c.liveRunnerId === chat.liveRunnerId &&
            chatReadMark(wsId, c.id) > readTicket,
        )
        if (newerClaimant) {
          const landed = s.agentChats.chats[idx === -1 ? s.agentChats.chats.length - 1 : idx]
          if (landed) {
            landed.liveRunnerId = ''
            landed.terminalSessionId = ''
          }
          return
        }
      }

      // terminalWaits is deliberately NOT written here, exactly as `working` is
      // not: both are frame-driven maps, and a single-chat REFETCH is a snapshot
      // taken at the moment it was issued, not at the moment it lands.
      //
      // The race is real. A spawn emits `started` (which refetches) and the CLI
      // then puts up its trust dialog a second later, which emits `terminal_wait`.
      // If the older refetch resolved last, its "nothing is blocking this chat"
      // would overwrite the newer truth — and since the daemon publishes only on a
      // CHANGE, nothing would ever correct it. Repair comes from the authoritative
      // reseed instead (initial load and reconnect), which is what repairs a lost
      // `turn_stopped` too.

      if (!chat.liveRunnerId) return
      for (const c of s.agentChats.chats) {
        if (c.id !== chat.id && c.liveRunnerId === chat.liveRunnerId) {
          c.liveRunnerId = ''
          c.terminalSessionId = ''
        }
      }
    }),

  removeAgentChat: (chatId) => {
    set((s) => {
      s.agentChats.chats = s.agentChats.chats.filter((c) => c.id !== chatId)
      delete s.agentChats.working[chatId]
      delete s.agentChats.terminalWaits[chatId]
      delete s.agentChats.compacting[chatId]
      delete s.agentChats.settledPrompts[chatId]
      delete s.agentChats.streamingMessages[chatId]
      delete s.agentChats.turnRevision[chatId]
      delete s.agentChats.scrollPositions[chatId]
      s.agentChats.order = s.agentChats.order.filter((id) => id !== chatId)
      if (s.agentChats.activeChatId === chatId) s.agentChats.activeChatId = null
    })
    clearPersistedPromptQueue(get().workspaceId, chatId)
  },

  setAgentChatWorking: (chatId, working) =>
    set((s) => {
      s.agentChats.working[chatId] = working
      s.agentChats.turnRevision[chatId] = (s.agentChats.turnRevision[chatId] ?? 0) + 1
    }),

  setAgentChatTerminalWait: (chatId, wait) =>
    set((s) => {
      if (wait) s.agentChats.terminalWaits[chatId] = wait
      else delete s.agentChats.terminalWaits[chatId]
    }),

  setAgentChatCompacting: (chatId, active) =>
    set((s) => {
      if (active) s.agentChats.compacting[chatId] = true
      else delete s.agentChats.compacting[chatId]
    }),

  setAgentChatPromptSettled: (chatId, clientRequestId) =>
    set((s) => {
      const seen = s.agentChats.settledPrompts[chatId] ?? []
      if (seen.includes(clientRequestId)) return
      s.agentChats.settledPrompts[chatId] = [...seen, clientRequestId].slice(
        -SETTLED_PROMPTS_PER_CHAT,
      )
    }),

  setAgentChatStreamingMessage: (chatId, message) =>
    set((s) => {
      if (!message) {
        delete s.agentChats.streamingMessages[chatId]
        return
      }
      // In-place upsert on the Immer draft, not a rebuild — the array is
      // expected to hold at most a handful of concurrently-open items, so
      // this find is O(open items), and mutating avoids reallocating the
      // array on every token the way `.map()`/spread would.
      const list = (s.agentChats.streamingMessages[chatId] ??= [])
      const existing = list.find((m) => m.id === message.id)
      if (existing) existing.text = message.text
      else list.push(message)
    }),

  pruneAgentChatStreamingMessages: (chatId, ids) =>
    set((s) => {
      if (ids.length === 0) return
      const list = s.agentChats.streamingMessages[chatId]
      if (!list) return
      const drop = new Set(ids)
      const kept = list.filter((m) => !drop.has(m.id))
      if (kept.length === list.length) return
      if (kept.length === 0) delete s.agentChats.streamingMessages[chatId]
      else s.agentChats.streamingMessages[chatId] = kept
    }),

  setAgentChatScrollPosition: (chatId, position) =>
    set((s) => {
      s.agentChats.scrollPositions[chatId] = position
    }),

  setAgentChatSelection: (chatId, model, effort) =>
    set((s) => {
      const chat = s.agentChats.chats.find((c) => c.id === chatId)
      if (!chat) return
      // Both halves, always — they are one answer. The endpoint takes the whole
      // selection because which effort levels are valid is a property of the
      // model, and writing one half here would let the store hold a pair that was
      // never jointly sent.
      chat.model = model
      chat.effort = effort
    }),

  setAgentChatOrder: (order) => {
    saveOrder(get().workspaceId, order)
    set((s) => {
      s.agentChats.order = order
    })
  },

  hydrateAgentChatOrder: () =>
    set((s) => {
      s.agentChats.order = loadOrder(get().workspaceId)
    }),

  setActiveAgentChatId: (chatId) =>
    set((s) => {
      s.agentChats.activeChatId = chatId
    }),

  setAgentProviders: (providers) =>
    set((s) => {
      s.agentChats.providers = providers
    }),

  seedAgentChatFolders: (folders) =>
    set((s) => {
      s.agentChats.folders = folders
    }),

  applyAgentChatFolders: (folders) =>
    set((s) => {
      const at = new Map(s.agentChats.folders.map((f, i) => [f.id, i]))
      for (const folder of folders) {
        const i = at.get(folder.id)
        if (i === undefined) {
          at.set(folder.id, s.agentChats.folders.length)
          s.agentChats.folders.push(folder)
        } else {
          s.agentChats.folders[i] = folder
        }
      }
    }),

  removeAgentChatFolder: (folderId) =>
    set((s) => {
      const gone = s.agentChats.folders.find((f) => f.id === folderId)
      if (!gone) return
      const grandparent = gone.parentId ?? ''
      s.agentChats.folders = s.agentChats.folders.filter((f) => f.id !== folderId)
      for (const f of s.agentChats.folders) {
        if ((f.parentId ?? '') === folderId) f.parentId = grandparent
      }
      for (const c of s.agentChats.chats) {
        if ((c.parentId ?? '') === folderId) c.parentId = grandparent
      }
    }),

  setAgentChatPlacement: (chatId, parentId, order) =>
    set((s) => {
      const chat = s.agentChats.chats.find((c) => c.id === chatId)
      if (!chat) return
      chat.parentId = parentId
      chat.order = order
    }),
})

import type {
  AgentChat,
  AgentChatFolder,
  AgentTelemetry,
  AgentTerminalWait,
} from '@/features/agent/api/agent-chat-reads'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { ChatFrame, ChatFrameOutcome } from '@/features/agent/lib/reduce-chat-frame'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

export interface AgentChatsState {
  chats: AgentChat[]
  /**
   * Has an AUTHORITATIVE chat list ever landed for this workspace?
   *
   * `chats` alone cannot answer that, and the difference is not cosmetic. An
   * empty-or-incomplete list reads identically before the first seed and after
   * one that simply does not mention some chat — but "not known YET" is a wait
   * that ends, and "not in the list" is a wait that does not. A surface pointed
   * at a chat the list never mentions (a pane restored from a saved layout, a
   * chat opened from another scope) has to be able to tell those apart, or it
   * sits in its pre-resolution state for the life of the mount.
   *
   * Set once by seedAgentChats and never cleared: this is "the answer arrived",
   * not "the answer is current".
   */
  listSeeded: boolean
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
   * Client request ids the daemon has reported as over WITHOUT any proof the
   * provider ever took them — its delivery timeout expiring, and nothing else.
   *
   * Kept apart from settledPrompts because the two call for opposite treatment.
   * A settled prompt is spent and its queue item can go; an abandoned one is a
   * prompt that may never have been seen at all, and its queue item holds the
   * only surviving copy of what the user typed — the daemon's journal records a
   * hash of the text and never the text itself, and nothing reached the ledger.
   * Dropping those ids the way settled ones are dropped is what silently erased
   * a user's message after an idle gap.
   *
   * Bounded per chat exactly like settledPrompts, and for the same reason.
   */
  abandonedPrompts: Record<string, string[]>
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
  streamingMessages: Record<string, { id: string; text: string; startedAt?: string }[]>
  /**
   * What the agent is THINKING right now, keyed by chat — the current thinking
   * block only, never a history.
   *
   * Separate from streamingMessages on purpose. A thought is not an answer: it is
   * never recorded in the ledger, so useChatMessages' prune-against-the-ledger
   * pass could never retire it, and rendering it as an assistant bubble would put
   * the model's reasoning into the transcript as though it had said it out loud.
   *
   * A reasoning model spends most of a hard turn here emitting nothing else, so
   * this is the difference between a chat that looks busy and one that looks dead.
   * Cleared on every turn edge — it belongs to the turn that produced it.
   */
  streamingReasoning: Record<string, { id: string; text: string }>
  /**
   * A running tool's output as it is produced, keyed by chat — the newest tool
   * only. `id` is the tool call it belongs to, so a client can put the lines
   * under the right running row.
   *
   * Live-only for the same reason as streamingReasoning: the tool's FULL output
   * lands once, on the completed tool call, and keeping every line of every
   * build in the ledger would multiply the transcript for text nobody scrolls
   * back to. Cleared on every turn edge.
   */
  streamingToolOutput: Record<string, { id: string; text: string }>
  /**
   * The agent's own running to-do list for the current turn, keyed by chat.
   *
   * Restated WHOLESALE by the server on every update — the newest list is the
   * entire truth, so this is a replace, never a merge, and there is no ordering
   * or diffing for a client to get wrong. Live-only and cleared at the turn edge,
   * for the same reason as streamingReasoning: a plan for a turn in progress is a
   * view of it, not a record.
   */
  streamingPlan: Record<string, { text: string; status: string }[]>
  /** The newest complete unified diff for the current turn. */
  streamingDiff: Record<string, { id: string; text: string }>
  /**
   * The provider's newest usage report per chat — context, rate limits, cost.
   * Written by the `telemetry` frame the daemon pushes as each report lands, and
   * once by the gauge's own first read; never polled.
   */
  telemetry: Record<string, AgentTelemetry>
  /**
   * The prompts each chat is BLOCKED on right now — permissions, questions,
   * elicitations — keyed by chat.
   *
   * Written whole by the `choice` frame (and by the snapshot a connecting socket
   * is sent), so a chat that was hidden when one opened already holds it the
   * first time it is shown. Absent means none; resolved prompts are history and
   * come from the activity read, never from here.
   */
  choices: Record<string, AgentChoice[]>
  /** Monotonic notification counter. It advances for every server turn state
   *  write even when React batches a fast true→false pair into one render, and
   *  on an authoritative reconnect reseed because a complete idle→idle turn
   *  may have occurred while the socket was down. */
  turnRevision: Record<string, number>
  /**
   * A one-shot "open the takeover with this scene" signal, keyed by chat id —
   * how an Edit button on a diagram rendered deep in a chat's transcript
   * reaches the composer (which owns the takeover) without threading a
   * callback prop through every Plate node component in between. The
   * composer's effect consumes it (opens the takeover, preloaded) and clears
   * it in the same tick; never persisted.
   */
  excalidrawEditRequests: Record<string, ParsedExcalidrawScene>
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
  /**
   * Apply an authoritative list read: every row under the version rule
   * (reduce-chat-frame.ts), and mark the list seeded. Returns the ids the
   * store holds that the list does not mention — SUSPECTS, not deletions: a
   * repo-scoped list omits a project-home chat that still exists, and a chat
   * created after the read was served is newer than the list. The caller
   * confirms each before forgetting it.
   */
  seedAgentChats: (chats: AgentChat[]) => string[]
  /** Invalidate every mounted chat transcript after a socket outage. This is
   * independent of the reconnect GET so even a failed/superseded read cannot
   * hide a complete idle-to-idle turn that happened while disconnected. */
  notifyAgentChatMessages: () => void
  /** Apply one chat snapshot from a single-chat read, under the version rule. */
  applyAgentChat: (chat: AgentChat) => ChatFrameOutcome
  /** Apply one chat frame off the feed — the ONE writer of chat state. */
  applyAgentChatFrame: (frame: ChatFrame) => ChatFrameOutcome
  removeAgentChat: (chatId: string) => void
  /** Write — or clear, with false — whether a chat is LIVE mid-compaction
   *  right now. See AgentChatsState.compacting for why this must never be
   *  derived from the ledger. */
  setAgentChatCompacting: (chatId: string, active: boolean) => void
  /** Record that one delivered prompt is over without having produced a turn. */
  setAgentChatPromptSettled: (chatId: string, clientRequestId: string) => void
  /** Record one prompt the daemon retired with no proof the provider took it.
   *  The queue KEEPS its text and surfaces a failure — see abandonedPrompts. */
  setAgentChatPromptAbandoned: (chatId: string, clientRequestId: string) => void
  /** Upsert (by id) one message a chat is mid-way through saying, or clear
   *  ALL of a chat's in-flight messages with null (a new turn starting). */
  setAgentChatStreamingMessage: (
    chatId: string,
    message: { id: string; text: string; startedAt?: string } | null,
  ) => void
  /** Replace (or clear, with null) the thinking block a chat is mid-way through.
   *  Only the latest is kept — see AgentChatsState.streamingReasoning. */
  setAgentChatStreamingReasoning: (
    chatId: string,
    block: { id: string; text: string } | null,
  ) => void
  /** Replace (or clear, with null) the live output of the tool a chat is
   *  currently running. See AgentChatsState.streamingToolOutput. */
  setAgentChatStreamingToolOutput: (
    chatId: string,
    block: { id: string; text: string } | null,
  ) => void
  /** Replace (or clear, with null) the agent's running to-do list for a chat.
   *  Always a whole list — see AgentChatsState.streamingPlan. */
  setAgentChatStreamingPlan: (
    chatId: string,
    steps: { text: string; status: string }[] | null,
  ) => void
  setAgentChatStreamingDiff: (chatId: string, diff: { id: string; text: string } | null) => void
  /** Replace (or clear, with null) the provider's newest usage report. */
  setAgentChatTelemetry: (chatId: string, report: AgentTelemetry | null) => void
  /** Replace the prompts a chat is blocked on; an empty list clears them. */
  setAgentChatChoices: (chatId: string, choices: AgentChoice[]) => void
  /** Forget every chat's prompts. A reconnect does this before the new socket's
   *  snapshot restates the ones still pending, so none that resolved during the
   *  outage survives it. */
  resetAgentChatChoices: () => void
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
  /** Signal the composer to open the takeover, preloaded with this scene —
   *  see AgentChatsState.excalidrawEditRequests. */
  requestExcalidrawEdit: (chatId: string, scene: ParsedExcalidrawScene) => void
  /** Consume (drop) a chat's pending edit request once the composer has acted on it. */
  clearExcalidrawEditRequest: (chatId: string) => void
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
  listSeeded: false,
  working: {},
  terminalWaits: {},
  compacting: {},
  settledPrompts: {},
  abandonedPrompts: {},
  streamingMessages: {},
  streamingReasoning: {},
  streamingToolOutput: {},
  streamingPlan: {},
  streamingDiff: {},
  telemetry: {},
  choices: {},
  turnRevision: {},
  excalidrawEditRequests: {},
  order: [],
  activeChatId: null,
  providers: [],
  folders: [],
}

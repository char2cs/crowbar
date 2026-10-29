import type {
  AgentChat,
  AgentChatFolder,
  AgentTelemetry,
  AgentTerminalWait,
} from '@/features/agent/api/agent-chat-reads'
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
   * The provider's newest usage report per chat — context, rate limits, cost.
   * Written by the `telemetry` frame the daemon pushes as each report lands, and
   * once by the gauge's own first read; never polled.
   */
  telemetry: Record<string, AgentTelemetry>
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
  /** Apply one chat snapshot from a single-chat read, under the version rule. */
  applyAgentChat: (chat: AgentChat) => ChatFrameOutcome
  /** Apply one chat frame off the feed — the ONE writer of chat state. */
  applyAgentChatFrame: (frame: ChatFrame) => ChatFrameOutcome
  removeAgentChat: (chatId: string) => void
  /** Replace (or clear, with null) the provider's newest usage report. */
  setAgentChatTelemetry: (chatId: string, report: AgentTelemetry | null) => void
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
  telemetry: {},
  excalidrawEditRequests: {},
  order: [],
  activeChatId: null,
  providers: [],
  folders: [],
}

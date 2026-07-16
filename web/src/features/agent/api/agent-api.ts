import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

// Workspace-scoped agentic-chat REST client. Routes nest under
// workspaceBase(wsId)/agent (00 agentic-engine spec §2); the {success,data}
// envelope is unwrapped by apiFetch. Modelled on features/git/api/review-api.ts.

function agentBase(wsId: string): string {
  return `${workspaceBase(wsId)}/agent`
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────

/** One conversation a chat has hosted — append-only history, oldest first. It is
 *  what a "segment" really was, minus everything that described a process (no
 *  status, no PTY, no runner id), so it cannot drift from reality. */
export interface ChatConversation {
  chatId: string
  providerId: string
  sessionId: string
  firstSeenAt: string
}

// AgentChat carries three DERIVED runner facts the backend joins on at read time;
// none of them is stored on the chat.
//
// liveRunnerId is the WHOLE liveness contract. It names the runner (the vendor-CLI
// process) currently placed on this chat, and it is '' exactly when the chat is
// dormant — nobody is talking to it, because its CLI exited or died with the
// daemon. There is deliberately no status/isLive flag anywhere on this shape: a
// second authority on liveness could only drift from the process, and that drift
// is the production bug this model exists to delete. A live-runner row exists
// exactly while its PTY does, so the PRESENCE of liveRunnerId IS the answer — a
// client needs no second call to confirm it.
export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  /** The runner placed on this chat, or '' when the chat is dormant. */
  liveRunnerId: string
  /** That runner's PTY — what a chat pane attaches to. '' exactly when liveRunnerId is. */
  terminalSessionId: string
  /** The live runner's provider, else the provider of the chat's LAST conversation
   *  (so a dormant chat still shows the right glyph, and Resume knows who to bring
   *  back). '' only on a chat no runner has ever been placed on. */
  activeProviderId: string
  createdAt: string
}

export interface AgentChatDetail extends AgentChat {
  conversations: ChatConversation[]
}

export interface AgentProvider {
  id: string
  displayName: string
  icon: string
}

// ── Mappers (wire → store types). Identity today, but kept explicit so a
//    future wire/store divergence changes one place (review-api idiom). ──
function mapChat(c: AgentChat): AgentChat {
  return {
    id: c.id,
    workspaceId: c.workspaceId,
    title: c.title,
    liveRunnerId: c.liveRunnerId,
    terminalSessionId: c.terminalSessionId,
    activeProviderId: c.activeProviderId,
    createdAt: c.createdAt,
  }
}

// ── Reads ───────────────────────────────────────────────────────────
export async function listChats(wsId: string): Promise<AgentChat[]> {
  const raw = await apiFetch<AgentChat[]>(`${agentBase(wsId)}/chats`)
  return (raw ?? []).map(mapChat)
}

export async function getChat(wsId: string, id: string): Promise<AgentChatDetail> {
  const raw = await apiFetch<AgentChatDetail>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`)
  return { ...mapChat(raw), conversations: raw.conversations ?? [] }
}

export async function listProviders(wsId: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`${agentBase(wsId)}/providers`)
  return raw ?? []
}

// ── Writes ──────────────────────────────────────────────────────────
export async function createChat(wsId: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${agentBase(wsId)}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider }),
  })
  return res.id
}

// switchProvider quits the chat's current vendor CLI, hands off the accumulated
// context, and starts `provider` as a NEW RUNNER on the same chat. Returns that
// runner's id — the chat is unchanged, the process is not.
export async function switchProvider(wsId: string, id: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${agentBase(wsId)}/chats/${encodeURIComponent(id)}/switch`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider }),
    },
  )
  return res.id
}

// resumeChat revives a DORMANT chat — one no runner points at, because its CLI
// exited or died with the daemon (agent PTYs are never persisted, so a restart
// always takes them). The backend brings the chat's last provider back into its
// own native session, so the conversation continues exactly where it left off.
//
// Returns the id of the RUNNER now on the chat. A chat that is still live is a
// no-op that hands back the runner already there, so this can never end up with
// two CLIs on one conversation.
export async function resumeChat(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${agentBase(wsId)}/chats/${encodeURIComponent(id)}/resume`,
    { method: 'POST' },
  )
  return res.id
}

// stopChat gracefully terminates the chat's live vendor CLI and leaves the chat
// DORMANT and resumable — the counterpart of resumeChat. It is what closing a
// chat TAB calls: the agent process stops, but the chat entry and its bound
// conversation are KEPT, so reopening the tab revives the real conversation
// through the same resume path. This is NOT deleteChat: the chat is preserved.
// A chat whose CLI is already gone is a backend no-op.
export async function stopChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

export async function renameChat(wsId: string, id: string, title: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export async function deleteChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

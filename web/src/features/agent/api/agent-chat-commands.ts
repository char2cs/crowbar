import { apiFetch } from '@/lib/api/fetch'
import {
  chatBase,
  mapChat,
  mapShift,
  type AgentChat,
  type AgentChatFolder,
  type FolderShift,
} from '@/features/agent/api/agent-chat-reads'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { chatBaseOn } from '@/lib/workspace-scope-url'

export interface AgentPromptResult {
  runnerId: string
  terminalSessionId: string
}

// Answers a prompt the CLI is blocked on. `optionIds` is one flat list across all questions:
// non-empty, one kind (never allow+deny) and covering every question, else 400. Elicitations
// answer accept/decline/cancel + `content`. Show 400 and 409 (CLI now asking in its terminal).
export async function answerChoice(
  wsId: string,
  chatId: string,
  choiceId: string,
  answer: { optionIds: string[]; reason?: string; content?: unknown },
): Promise<void> {
  await apiFetch<unknown>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}` +
      `/choices/${encodeURIComponent(choiceId)}/answer`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      // An omitted reason/content is omitted on the wire, not sent as null: the
      // backend reads content as raw JSON and "absent" is its own answer there.
      body: JSON.stringify({
        optionIds: answer.optionIds,
        reason: answer.reason,
        content: answer.content,
      }),
    },
  )
}

/** Restart the provider TUI with a completed prompt; `clientRequestId` is stable across retries.
 *  Optional args are the composer's staged pick, committed atomically here (switching provider if
 *  it differs). Omit model+effort to keep the sticky pick; '' resets it to the provider default. */
export async function submitAgentPrompt(
  wsId: string,
  id: string,
  text: string,
  clientRequestId: string,
  provider?: string,
  model?: string,
  effort?: string,
  /** The refused prompt this send retries: the daemon links the two in the ledger. */
  retryOf?: string,
): Promise<AgentPromptResult> {
  return apiFetch<AgentPromptResult>(`${chatBase(wsId)}/${encodeURIComponent(id)}/prompts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, clientRequestId, provider, model, effort, retryOf }),
  })
}

/** What a retract did: withdrew a prompt the provider never took, or ended its turn. */
export interface RetractAnswer {
  state: 'retracted' | 'interrupted'
  text: string
}

/** Esc after a send: 409 once the prompt settled or its turn closed, 404 for an id never seen. */
export async function retractPrompt(
  wsId: string,
  chatId: string,
  requestId: string,
): Promise<RetractAnswer> {
  return apiFetch<RetractAnswer>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}/prompts/${encodeURIComponent(requestId)}/retract`,
    { method: 'POST' },
  )
}

/** Override ONE chat's approval level. Write-only: there is no GET for a chat's level, so there is
 *  nothing to reconcile against. */
export async function setChatPermissionLevel(
  wsId: string,
  chatId: string,
  level: PermissionLevel,
): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/permission-level`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ level }),
  })
}

// switchProvider quits the chat's current vendor CLI, hands off the accumulated
// context, and starts `provider` as a NEW RUNNER on the same chat. Returns that
// runner's id — the chat is unchanged, the process is not. `surface` is the view
// the new runner starts on; omitted keeps the chat's own.
export async function switchProvider(
  wsId: string,
  id: string,
  provider: string,
  surface?: LandingChatPresentation,
): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/switch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider, ...(surface && { surface }) }),
  })
  return res.id
}

// resumeChat revives a DORMANT chat (agent PTYs never survive a daemon restart), bringing its
// last provider back into its own native session. Returns the runner now on it; a live chat is a
// no-op returning its existing runner, so a conversation never gets two CLIs.
export async function resumeChat(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/resume`, {
    method: 'POST',
  })
  return res.id
}

// compactChat triggers the provider's declared compaction gesture (a slash command or an API
// call); the context is the provider's, not Crowbar's. 404 when none is declared, so gate the
// control on the provider's `compaction` capability.
export async function compactChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/compact`, {
    method: 'POST',
  })
}

/** Type the descriptor-declared reply into the prompt a chat's CLI is parked on.
 *  The wait itself clears when the daemon next sees the prompt gone; a 409 means it already has. */
export async function answerTerminalWait(
  wsId: string,
  chatId: string,
  answerId: string,
): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/terminal-wait/answer`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ answerId }),
  })
}

// stopChat is the Stop button: it interrupts the turn in place where the provider allows,
// otherwise ends the CLI, leaving the chat DORMANT and resumable. A CLI already gone is a no-op.
export async function stopChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

// killChat force-ends the chat's live vendor CLI on hostId, mid-turn included — unlike stopChat,
// which interrupts a turn in place. The chat entry stays, dormant and resumable.
export async function killChat(hostId: string, chatId: string): Promise<void> {
  await apiFetch<unknown>(`${chatBaseOn(hostId, chatId)}/kill`, { method: 'POST' })
}

// switchToTerminal moves the chat onto its provider's TUI (handed over, or relaunched on the
// resume ladder). Returns its terminal session ('' for a dormant chat, which only records the
// move). 409 while a turn is in flight; 422 for a provider with no TUI.
export async function switchToTerminal(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/switch-to-terminal`,
    { method: 'POST' },
  )
  return res.id
}

// switchToNative moves the chat onto Crowbar's own chat surface; a chat already
// there is a backend no-op.
export async function switchToNative(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/switch-to-native`, {
    method: 'POST',
  })
}

export async function renameChat(wsId: string, id: string, title: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

/** Withdraws the chat's pending move: it stays where it runs. */
export async function cancelChatMove(wsId: string, chatId: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/move/cancel`, {
    method: 'POST',
  })
}

/** Makes a thread a branch of its own, cut from its folder's HEAD at its turn boundary; the
 *  daemon names the branch. */
export async function branchOutChat(wsId: string, chatId: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/branch-out`, {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

/** Deletes a chat and the threads hanging off it (a thread without its parent reads a context
 *  that is gone); resolves with the chats its git cascade also took. `init` carries `keepalive`
 *  for the removal tray's pagehide flush, so an undo window cut short by a reload still deletes. */
export async function deleteChat(wsId: string, id: string, init?: RequestInit): Promise<string[]> {
  const raw = await apiFetch<{ takenChatIds?: unknown } | undefined>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}`,
    { method: 'DELETE', ...init },
  )
  const taken = raw?.takenChatIds
  return Array.isArray(taken) ? taken.filter((id): id is string => typeof id === 'string') : []
}

/** Move a chat: under `parentId` (a chat, a folder, or '' for the root) at `order`. A
 *  `workspaceId` other than `wsId` (the chat's own) asks for a move there at the chat's turn
 *  boundary: the answer is the chat unchanged, carrying `pendingMove`. */
export async function setChatPlacement(
  wsId: string,
  chatId: string,
  patch: { parentId?: string; order?: number; workspaceId?: string; label?: string },
): Promise<{ chat: AgentChat; shifted: AgentChatFolder[] }> {
  const raw = await apiFetch<{ chat: AgentChat } & Partial<FolderShift>>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}/placement`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    },
  )
  return { chat: mapChat(raw.chat), shifted: mapShift(raw) }
}

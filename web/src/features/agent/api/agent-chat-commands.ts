import { apiFetch } from '@/lib/api/fetch'
import { clearPersistedPromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
import {
  chatBase,
  mapChat,
  mapShift,
  type AgentChat,
  type AgentChatFolder,
  type FolderShift,
} from '@/features/agent/api/agent-chat-reads'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'

export interface AgentPromptResult {
  runnerId: string
  terminalSessionId: string
}

/**
 * Answer a prompt the provider CLI is blocked on, from the chat.
 *
 * `optionIds` is ONE FLAT LIST across the whole prompt, however many questions it
 * asks: the ids say which question each pick answers, so a three-question prompt
 * is still one call. It must name at least one option (400 otherwise), the picks
 * must all be of ONE kind — "allow" and "deny" together are not an answer and no
 * template could render them — and they must COVER EVERY QUESTION. A partial
 * answer is refused with 400 rather than sent, because a CLI handed answers to two
 * of three questions goes on waiting for the third with nothing able to send it.
 *
 * A prompt that offers NO options is an elicitation, whose answer is the
 * provider's own verb — `accept`, `decline`, `cancel` — with the filled-in form in
 * `content`.
 *
 * Two failures are the caller's to SHOW rather than swallow. 400 is a decision
 * this provider cannot express (claude's `suggestion` options are the shipped
 * example: the backend declares no template for them rather than silently
 * narrowing one to a plain allow). 409 is no relay holding the gate any more — by
 * then the CLI is asking at its own terminal, and reporting success would be a lie.
 */
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

/** Ask Crowbar to restart the same interactive provider TUI with a completed
 *  prompt. `clientRequestId` is stable across retries.
 *
 *  `provider`/`model`/`effort` are the composer's STAGED pick, if the picker
 *  has one. OMIT model/effort when nothing is staged, which leaves the chat's
 *  sticky selection exactly as it was; passing '' is NOT the same thing — it
 *  is a real pick of the provider's own default, and the daemon clears the
 *  chat back to it. (Provider has no such reading: a chat always runs some
 *  provider, so '' there simply means nothing staged.) The two travel as one
 *  pair, so a body carrying only one half clears the other.
 *  A staged pick is committed on THIS call, atomically with the prompt: the
 *  picker itself never writes selection or switches provider on its own, so
 *  choosing a row never mutates the chat until the user actually sends. A
 *  provider that differs from the chat's current one is switched to (killing
 *  the outgoing CLI and spawning the new one) as part of this same request —
 *  see the backend's Usecase.SubmitPrompt for the exact ordering. An invalid
 *  pair is a 400 (outside the provider's catalogue) or a 422 (no provider yet). */
export async function submitAgentPrompt(
  wsId: string,
  id: string,
  text: string,
  clientRequestId: string,
  provider?: string,
  model?: string,
  effort?: string,
): Promise<AgentPromptResult> {
  return apiFetch<AgentPromptResult>(`${chatBase(wsId)}/${encodeURIComponent(id)}/prompts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, clientRequestId, provider, model, effort }),
  })
}

/** Override ONE chat's own approval level, independent of the global default
 *  above. Write-only — there is no GET for a single chat's level, so a caller
 *  has nothing to reconcile against and must not pretend otherwise. */
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
// runner's id — the chat is unchanged, the process is not.
export async function switchProvider(wsId: string, id: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/switch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider }),
  })
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
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/resume`, {
    method: 'POST',
  })
  return res.id
}

// compactChat asks the chat's CLI to compact its OWN context. Crowbar cannot compact
// anything itself — the context belongs to the provider — so this triggers whatever
// gesture the provider declares: claude has no API for it and gets its /compact slash
// command, an API-transport provider gets the equivalent call.
//
// A provider that declares no gesture answers 404. Gate the control on the provider's
// `compaction` capability rather than letting the user press something that cannot work.
export async function compactChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/compact`, {
    method: 'POST',
  })
}

// stopChat is the chat's Stop button: it interrupts the running turn in place
// where the provider allows it and otherwise ends the CLI, leaving the chat
// DORMANT and resumable — the counterpart of resumeChat. This is NOT deleteChat.
// A chat whose CLI is already gone is a backend no-op.
export async function stopChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

// closeChat ends the chat's live vendor CLI because no view displays the chat
// any more, mid-turn included — unlike stopChat, which interrupts a turn in
// place and may leave the CLI running. The chat entry stays, dormant and
// resumable.
export async function closeChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/close`, {
    method: 'POST',
  })
}

// switchToTerminal moves the chat onto its provider's own TUI: the daemon hands
// the session over, or relaunches the TUI on the resume ladder (transcript when
// the provider cannot load its own session yet). Returns the TUI's terminal
// session ('' for a dormant chat, which only records the move). Rejects (409)
// while a turn is in flight and (422) for a provider with no TUI to show.
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

// Deleting a chat CASCADES to the threads hanging off it — a thread exists to
// continue the conversation above it, and leaving it behind strands it reading a
// context that no longer exists.
//
// `init` exists for one caller: the removal tray's pagehide flush, which sends
// with `keepalive` so an eight-second undo window that ends in a reload still
// posts the delete the user already walked away from.
export async function deleteChat(wsId: string, id: string, init?: RequestInit): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    ...init,
  })
  clearPersistedPromptQueue(wsId, id)
}

/** Move a chat: under `parentId` (a chat, a folder, or '' for the root) at `order`. */
export async function setChatPlacement(
  wsId: string,
  chatId: string,
  patch: { parentId?: string; order?: number },
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

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

export async function answerChoice(
  wsId: string,
  chatId: string,
  choiceId: string,
  answer: { optionIds: string[]; content?: unknown },
): Promise<void> {
  await apiFetch<unknown>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}` +
      `/choices/${encodeURIComponent(choiceId)}/answer`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        optionIds: answer.optionIds,
        content: answer.content,
      }),
    },
  )
}

export async function submitAgentPrompt(
  wsId: string,
  id: string,
  text: string,
  clientRequestId: string,
  provider?: string,
  model?: string,
  effort?: string,
  retryOf?: string,
): Promise<AgentPromptResult> {
  return apiFetch<AgentPromptResult>(`${chatBase(wsId)}/${encodeURIComponent(id)}/prompts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, clientRequestId, provider, model, effort, retryOf }),
  })
}

export interface RetractAnswer {
  state: 'retracted' | 'interrupted'
  text: string
}

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

export async function resumeChat(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/resume`, {
    method: 'POST',
  })
  return res.id
}

export async function compactChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/compact`, {
    method: 'POST',
  })
}

export async function stopShell(wsId: string, id: string, shellId: string): Promise<void> {
  await apiFetch<unknown>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/shells/${encodeURIComponent(shellId)}/stop`,
    { method: 'POST' },
  )
}

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

export async function stopChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

export async function killChat(hostId: string, chatId: string): Promise<void> {
  await apiFetch<unknown>(`${chatBaseOn(hostId, chatId)}/kill`, { method: 'POST' })
}

export async function switchToTerminal(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/switch-to-terminal`,
    { method: 'POST' },
  )
  return res.id
}

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

export async function cancelChatMove(wsId: string, chatId: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/move/cancel`, {
    method: 'POST',
  })
}

export async function branchOutChat(wsId: string, chatId: string, branch = ''): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/branch-out`, {
    method: 'POST',
    body: JSON.stringify({ branch }),
  })
}

export async function deleteChat(wsId: string, id: string, init?: RequestInit): Promise<string[]> {
  const raw = await apiFetch<{ takenChatIds?: unknown } | undefined>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}`,
    { method: 'DELETE', ...init },
  )
  const taken = raw?.takenChatIds
  return Array.isArray(taken) ? taken.filter((id): id is string => typeof id === 'string') : []
}

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

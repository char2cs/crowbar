import { useStore } from 'zustand'
import type { AgentChat, AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { entityOf, entityStore } from '@/core/entities/entity-store'
import { useChatHost } from '@/lib/host-resolve'
import { selectAgentChat, useSelect } from '@/core/entities/select'

export function useChatField<T>(chatId: string, pick: (chat: AgentChat | undefined) => T): T {
  return useSelect((s) => pick(selectAgentChat(s, chatId)))
}

export function useChatWorking(chatId: string): boolean {
  return useChatField(chatId, (c) => c?.working ?? false)
}

export function useChatTelemetry(chatId: string): AgentTelemetry | null {
  const hostId = useChatHost(chatId)
  return useStore(entityStore, (s) =>
    hostId ? (entityOf(s, 'telemetry', hostId, chatId) ?? null) : null,
  )
}

export function limitResetsAt(telemetry: AgentTelemetry | null): string | undefined {
  const windows = (telemetry?.rateLimits ?? []).filter(
    (window) => window.resetsAt && window.usedPercent !== undefined,
  )
  if (windows.length === 0) return undefined
  return windows.reduce((worst, next) =>
    (next.usedPercent ?? 0) > (worst.usedPercent ?? 0) ? next : worst,
  ).resetsAt
}

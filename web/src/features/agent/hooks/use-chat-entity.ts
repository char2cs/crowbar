import { useStore } from 'zustand'
import type { AgentChat, AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { entityStore } from '@/core/entities/entity-store'
import { selectAgentChat, useSelect } from '@/core/entities/select'

/** One field of a chat entity; `pick` must return a primitive or a held reference. */
export function useChatField<T>(chatId: string, pick: (chat: AgentChat | undefined) => T): T {
  return useSelect((s) => pick(selectAgentChat(s, chatId)))
}

/** The daemon's folded busy flag for the chat, as its frames carry it. */
export function useChatWorking(chatId: string): boolean {
  return useChatField(chatId, (c) => c?.working ?? false)
}

/** The provider's newest usage report, which the chat's scope sends on subscribe and
 *  on every report after. */
export function useChatTelemetry(chatId: string): AgentTelemetry | null {
  return useStore(entityStore, (s) => s.byKind.telemetry[chatId]?.data ?? null)
}

/** When the most-consumed rate-limit window lifts: the window closest to
 *  spent is what stopped the turn, not the one that resets soonest. */
export function limitResetsAt(telemetry: AgentTelemetry | null): string | undefined {
  const windows = (telemetry?.rateLimits ?? []).filter(
    (window) => window.resetsAt && window.usedPercent !== undefined,
  )
  if (windows.length === 0) return undefined
  return windows.reduce((worst, next) =>
    (next.usedPercent ?? 0) > (worst.usedPercent ?? 0) ? next : worst,
  ).resetsAt
}

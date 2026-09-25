import { useEffect, useRef } from 'react'
import { getChatTelemetry, type AgentTelemetry } from '@/features/agent/api/agent-api'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

/**
 * The provider's own report of context, cost and rate limits.
 *
 * PUSHED, never polled: the daemon publishes every report on the chat feed as
 * it lands (the `telemetry` frame, about once a turn), and the stream writes it
 * into the workspace store. This hook reads that entry, and asks the daemon
 * ONCE — when the chat first becomes visible, or its surface changes — for the
 * report it already holds, because a chat that reported before this client
 * subscribed has no frame coming.
 *
 * Read by two consumers (the gauge and the composer, when a usage limit is what
 * stopped the turn). Nothing here is derived — a fresh session legitimately
 * reports no usage, and a confident 0% there would be a lie.
 */
export function useAgentTelemetry(
  wsId: string,
  chatId: string,
  visible: boolean,
): AgentTelemetry | null {
  const telemetry = useWorkspaceStoreById(wsId, (s) => s.agentChats.telemetry[chatId] ?? null)
  // The surface decides whether the chat carries a report at all, so a switch
  // is the one moment the held answer can be stale without a frame saying so.
  const surface = useWorkspaceStoreById(
    wsId,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.surface ?? '',
  )
  const readFor = useRef<string | null>(null)

  useEffect(() => {
    const key = `${chatId}\u0000${surface}`
    if (!visible || readFor.current === key) return
    readFor.current = key
    const controller = new AbortController()
    const read = async () => {
      try {
        const report = await getChatTelemetry(wsId, chatId, controller.signal)
        getOrCreateWorkspaceStore(wsId).getState().setAgentChatTelemetry(chatId, report)
      } catch {
        // A failed read leaves the last good gauge standing. It is an
        // indicator, not the conversation.
        if (readFor.current === key) readFor.current = null
      }
    }
    void read()
    return () => controller.abort()
  }, [wsId, chatId, visible, surface])

  return telemetry
}

/**
 * When the most-consumed rate-limit window lifts.
 *
 * The window that matters is the one closest to being spent, not the one that
 * resets soonest: a 7-day limit at 99% is what stopped the turn, and telling
 * someone about the 5-hour window that resets in ten minutes would send them
 * back to try again into the same wall.
 */
export function limitResetsAt(telemetry: AgentTelemetry | null): string | undefined {
  const windows = (telemetry?.rateLimits ?? []).filter(
    (window) => window.resetsAt && window.usedPercent !== undefined,
  )
  if (windows.length === 0) return undefined
  return windows.reduce((worst, next) =>
    (next.usedPercent ?? 0) > (worst.usedPercent ?? 0) ? next : worst,
  ).resetsAt
}

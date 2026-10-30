import { useEffect, useRef } from 'react'
import {
  getChatTelemetry,
  type AgentChat,
  type AgentTelemetry,
} from '@/features/agent/api/agent-chat-reads'
import { setChatTelemetry, useChatStore } from '@/features/agent/stores/chat-store'

/** One field of a chat entity; `pick` must return a primitive or a held reference. */
export function useChatField<T>(chatId: string, pick: (chat: AgentChat | undefined) => T): T {
  return useChatStore((s) => pick(s.chats[chatId]))
}

/** The daemon's folded busy flag for the chat, as its lifecycle frames carry it. */
export function useChatWorking(chatId: string): boolean {
  return useChatStore((s) => s.chats[chatId]?.working ?? false)
}

/**
 * The provider's newest usage report. Frames push every report; a chat that
 * reported before this client subscribed has none coming, so it is read once
 * when the chat becomes visible, and again when its surface changes.
 */
export function useChatTelemetry(
  wsId: string,
  chatId: string,
  visible: boolean,
): AgentTelemetry | null {
  const telemetry = useChatStore((s) => s.telemetry[chatId] ?? null)
  const surface = useChatStore((s) => s.chats[chatId]?.surface ?? '')
  const readFor = useRef<string | null>(null)

  useEffect(() => {
    const key = `${chatId}\u0000${surface}`
    if (!visible || readFor.current === key) return
    readFor.current = key
    const controller = new AbortController()
    getChatTelemetry(wsId, chatId, controller.signal)
      .then((report) => setChatTelemetry(chatId, report))
      // A failed read leaves the last good gauge standing.
      .catch(() => {
        if (readFor.current === key) readFor.current = null
      })
    return () => controller.abort()
  }, [wsId, chatId, visible, surface])

  return telemetry
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

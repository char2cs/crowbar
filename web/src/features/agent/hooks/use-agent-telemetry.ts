import { useEffect, useState } from 'react'
import { getChatTelemetry, type AgentTelemetry } from '@/features/agent/api/agent-api'

/** How often the provider's own report is re-read.
 *
 *  It is change-driven at the source — measured at roughly one report per turn,
 *  and two across a minute of idle — so this only has to be often enough that a
 *  gauge is not stale after a turn ends. */
const POLL_MS = 5000

/**
 * The provider's own report of context, cost and rate limits.
 *
 * Read ONCE per chat and shared, because two consumers want it: the gauge under
 * the composer, and the composer itself when a usage limit is what stopped the
 * turn. Nothing here is ever derived — a fresh session legitimately reports no
 * usage, because it is null until the first turn completes, and a confident 0%
 * there would be a lie.
 */
export function useAgentTelemetry(wsId: string, chatId: string, visible: boolean) {
  const [telemetry, setTelemetry] = useState<AgentTelemetry | null>(null)

  // Read by two independent consumers (the gauge and the composer, per the doc
  // comment above) rather than owned by one component that could key-remount on
  // chatId, so there is no caller-side key to replace this with.
  useEffect(() => {
    // react-doctor-disable-next-line react-doctor/no-adjust-state-on-prop-change
    setTelemetry(null)
  }, [chatId])

  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    const read = async () => {
      try {
        setTelemetry(await getChatTelemetry(wsId, chatId, controller.signal))
      } catch {
        // A telemetry read that fails leaves the last good gauge standing. It is
        // an indicator, not the conversation.
      }
    }
    void read()
    const timer = setInterval(() => void read(), POLL_MS)
    return () => {
      clearInterval(timer)
      controller.abort()
    }
  }, [wsId, chatId, visible])

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

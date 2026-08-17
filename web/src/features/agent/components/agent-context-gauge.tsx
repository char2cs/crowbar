import { useEffect, useState } from 'react'

import { type AgentTelemetry, getChatTelemetry } from '@/features/agent/api/agent-api'

/** How often the provider's own report is re-read.
 *
 *  It is change-driven at the source — measured at roughly one report per turn,
 *  and two across a minute of idle — so this only has to be often enough that a
 *  gauge is not stale after a turn ends. */
const POLL_MS = 5000

/** The provider's own report of how much context is left.
 *
 *  It renders NOTHING until the provider reports, and never derives a number it
 *  was not given: a fresh session legitimately has no gauge, because usage is
 *  null until the first turn completes, and a confident 0% there would be a lie.
 */
export function AgentContextGauge({
  wsId,
  chatId,
  visible,
}: {
  wsId: string
  chatId: string
  visible: boolean
}) {
  const [telemetry, setTelemetry] = useState<AgentTelemetry | null>(null)

  useEffect(() => {
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

  const used = telemetry?.context?.usedPercent
  if (used === undefined) return null

  return (
    <span
      className="text-muted-foreground text-xs tabular-nums"
      title={contextTitle(telemetry)}
      data-testid="agent-context-gauge"
    >
      {Math.round(used)}% context
    </span>
  )
}

function contextTitle(telemetry: AgentTelemetry | null): string {
  const context = telemetry?.context
  if (!context) return ''
  const parts: string[] = []
  if (context.usedTokens !== undefined && context.capacityTokens !== undefined) {
    parts.push(
      `${context.usedTokens.toLocaleString()} of ${context.capacityTokens.toLocaleString()} tokens`,
    )
  }
  for (const window of telemetry?.rateLimits ?? []) {
    if (window.usedPercent === undefined) continue
    parts.push(`${window.label || window.id}: ${Math.round(window.usedPercent)}%`)
  }
  const cost = telemetry?.cost?.totalUsd
  if (cost !== undefined) parts.push(`$${cost.toFixed(4)}`)
  return parts.join(' · ')
}

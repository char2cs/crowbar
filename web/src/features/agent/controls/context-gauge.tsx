import type { AgentTelemetry } from '@/features/agent/api/agent-api'
import { cn } from '@/lib/utils'

/** Past this, the bar warns — the point where a compaction is imminent and a
 *  long prompt is about to be a bad idea. */
const WARN_AT = 85

/**
 * How much of the model's context this chat has spent.
 *
 * Renders NOTHING until the provider reports. A provider that sends no usage
 * gets no gauge rather than an empty one, because "not reported" and "zero" are
 * different facts and a 0% bar over the first is a lie.
 */
export function AgentContextGauge({ telemetry }: { telemetry: AgentTelemetry | null }) {
  const used = telemetry?.context?.usedPercent
  if (used === undefined) return null
  const pct = Math.max(0, Math.min(100, used))

  return (
    <span className="gauge" title={contextTitle(telemetry)} data-testid="agent-context-gauge">
      <span className={cn('gbar', pct >= WARN_AT && 'warn')}>
        <span style={{ width: `${pct}%` }} />
      </span>
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

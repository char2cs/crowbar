import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { cn } from '@/lib/utils'

/** Past this, a compaction is imminent. */
const WARN_AT = 85

/**
 * No bar until the provider reports: "not reported" is not 0%. Compaction is an
 * unrelated capability, so it is still offered bar-less without a report.
 */
export function AgentContextGauge({
  telemetry,
  onCompact,
}: {
  telemetry: AgentTelemetry | null
  /** Absent when compaction can't be offered now: absence, never a disabled button. */
  onCompact?: () => void
}) {
  const used = telemetry?.context?.usedPercent
  if (used === undefined) {
    if (!onCompact) return null
    return (
      <button
        type="button"
        className="gauge chip compact-only"
        title="Compact the conversation"
        data-testid="agent-context-gauge"
        onClick={onCompact}
      >
        Compact
      </button>
    )
  }
  const pct = Math.max(0, Math.min(100, used))
  const bar = (
    <span className={cn('gbar', pct >= WARN_AT && 'warn')}>
      <span style={{ width: `${pct}%` }} />
    </span>
  )
  const title = contextTitle(telemetry)

  if (!onCompact) {
    return (
      <span className="gauge" title={title} data-testid="agent-context-gauge">
        {bar}
        {Math.round(used)}% context
      </span>
    )
  }

  return (
    <button
      type="button"
      className="gauge chip"
      title={title}
      data-testid="agent-context-gauge"
      onClick={onCompact}
    >
      {/* `.gaction` paints over the bar on hover so it never widens the row. */}
      <span className="gstack">
        {bar}
        <span className="gaction">Compact</span>
      </span>
      <span className="gpct">{Math.round(used)}% context</span>
    </button>
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

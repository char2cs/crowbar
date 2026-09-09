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
export function AgentContextGauge({
  telemetry,
  onCompact,
}: {
  telemetry: AgentTelemetry | null
  /**
   * The one gesture that spends less — offered here because this is the one
   * control on screen already reporting WHY someone would reach for it.
   * Absent means exactly what every other optional control in this bar means
   * (see provider-bar.tsx's own doc): the caller has already decided
   * compaction cannot be offered right now (the provider declares no
   * gesture, the chat is not live, or one is already running), so this stays
   * the plain, non-interactive gauge it always was rather than a disabled
   * button — the house rule is absence, never a greyed-out control.
   */
  onCompact?: () => void
}) {
  const used = telemetry?.context?.usedPercent
  if (used === undefined) return null
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
      {bar}
      {/* Swapped by CSS on hover (composer.css) — the same instant the row's
          other chips turn interactive, this one turns from a report into an
          offer instead of adding a second element beside it. */}
      <span className="gtext">
        <span className="gpct">{Math.round(used)}% context</span>
        <span className="gaction">Compact</span>
      </span>
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

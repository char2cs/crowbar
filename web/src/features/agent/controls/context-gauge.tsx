import type { AgentTelemetry } from '@/features/agent/api/agent-api'
import { cn } from '@/lib/utils'

/** Past this, the bar warns — the point where a compaction is imminent and a
 *  long prompt is about to be a bad idea. */
const WARN_AT = 85

/**
 * How much of the model's context this chat has spent, and the one gesture
 * that spends less.
 *
 * The BAR renders nothing until the provider reports — a provider that sends
 * no usage gets no gauge rather than an empty one, because "not reported" and
 * "zero" are different facts and a 0% bar over the first is a lie. COMPACTION
 * is a different, unrelated provider capability (AgentChatView's own
 * `onCompact` gate never looks at telemetry) and must not go hostage to a
 * report that has not arrived yet, or one a daemon restart wiped — so it is
 * offered on its own, bar-less, whenever `onCompact` is given and there is no
 * report to show alongside it.
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
  if (used === undefined) {
    // No usage report — never invent a 0% bar for it — but compaction is
    // still reachable on its own when the caller has decided to offer it.
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
      {/* `.gaction` stacks on the bar itself (composer.css `.gstack`, same
          grid-cell technique as before) — hidden until hover, then painted
          on top of the bar, never beside `.gpct` and never widening the row. */}
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

import { useCallback, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { Dropdown } from '@/components/ui/dropdown'
import { IS_MOBILE } from '@/utils/platform'

/** Past this, a compaction is imminent. */
const WARN_AT = 85

/** The footer's context-usage pill. Nothing until the provider reports: "not reported" is not 0%.
 *  Compaction is an unrelated capability, so `onCompact` offers it from the pill's menu (alone,
 *  without a report); absent means not offered, never a disabled control. */
export function AgentContextGauge({
  telemetry,
  onCompact,
}: {
  telemetry: AgentTelemetry | null
  onCompact?: () => void
}) {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const close = useCallback(() => setOpen(false), [])
  const used = telemetry?.context?.usedPercent

  if (used === undefined && !onCompact) return null
  const pct = used === undefined ? undefined : Math.max(0, Math.min(100, used))
  const body =
    pct === undefined ? (
      'Compact'
    ) : (
      <>
        <i className="ctx-ring" style={{ '--p': pct } as CSSProperties} />
        <b>{Math.round(pct)}%</b>
        {!IS_MOBILE && <span>context</span>}
      </>
    )
  const shared = {
    className: 'ctx-pill',
    'data-testid': 'agent-context-gauge',
    'data-level': pct !== undefined && pct >= WARN_AT ? 'warn' : undefined,
    title: contextTitle(telemetry),
  }
  if (!onCompact) return <span {...shared}>{body}</span>

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        {...shared}
        aria-haspopup={pct === undefined ? undefined : 'menu'}
        aria-expanded={pct === undefined ? undefined : open}
        onClick={() => (pct === undefined ? onCompact() : setOpen((was) => !was))}
      >
        {body}
      </button>
      <Dropdown
        isOpen={open}
        onClose={close}
        anchorRef={anchorRef}
        anchorSide="top"
        anchorAlign="end"
        items={[{ id: 'compact', label: 'Compact conversation', onClick: onCompact }]}
      />
    </>
  )
}

function contextTitle(telemetry: AgentTelemetry | null): string {
  const context = telemetry?.context
  if (!context) return 'Compact the conversation'
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

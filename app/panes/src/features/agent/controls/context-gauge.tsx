import { useCallback, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { Dropdown } from '@/components/ui/dropdown'
import { NativePlusMenuSheet } from '@/features/agent/composer/native-plus-menu'
import { hasNativeChrome } from '@/lib/native-chrome'
import { IS_MOBILE } from '@/utils/platform'

/** Past this, a compaction is imminent. */
const WARN_AT = 85

/** The footer's context-usage pill, in every chat's footer: a muted empty ring and just
 *  "context" until the provider reports, never a made-up 0%. While the
 *  conversation is being folded the ring is the warning's. A button when `onCompact` is
 *  offered (its menu, or at once without a report); absent means not offered, never disabled. */
export function AgentContextGauge({
  telemetry,
  compacting,
  onCompact,
}: {
  telemetry: AgentTelemetry | null
  compacting?: boolean
  onCompact?: () => void
}) {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const close = useCallback(() => setOpen(false), [])
  const used = telemetry?.context?.usedPercent

  const pct = used === undefined ? undefined : Math.max(0, Math.min(100, used))
  const body = (
    <>
      <i
        className="ctx-ring"
        data-unknown={pct === undefined || undefined}
        style={{ '--p': pct ?? 0 } as CSSProperties}
      />
      {pct !== undefined && <b>{Math.round(pct)}%</b>}
      {(pct === undefined || !IS_MOBILE) && <span>context</span>}
    </>
  )
  const shared = {
    className: 'ctx-pill',
    'data-testid': 'agent-context-gauge',
    'data-level': compacting || (pct !== undefined && pct >= WARN_AT) ? 'warn' : undefined,
    title: contextTitle(telemetry, !!onCompact),
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
      {hasNativeChrome() ? (
        <NativePlusMenuSheet
          open={open}
          title="Context"
          items={[
            {
              id: 'compact',
              title: 'Compact conversation',
              icon: 'ArrowClockwise',
              hint: null,
              divider: false,
            },
          ]}
          onPick={() => {
            close()
            onCompact()
          }}
          onClose={close}
        />
      ) : (
        <Dropdown
          isOpen={open}
          onClose={close}
          anchorRef={anchorRef}
          anchorSide="top"
          anchorAlign="end"
          items={[{ id: 'compact', label: 'Compact conversation', onClick: onCompact }]}
        />
      )}
    </>
  )
}

function contextTitle(telemetry: AgentTelemetry | null, canCompact: boolean): string {
  const context = telemetry?.context
  if (!context) return canCompact ? 'Compact the conversation' : 'Context usage not reported yet'
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

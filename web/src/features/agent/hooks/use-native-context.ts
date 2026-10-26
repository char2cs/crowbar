import { useEffect, useMemo, useRef } from 'react'
import type { AgentTelemetry } from '@/features/agent/api/agent-api'
import { onNativeChromeIntent, setNativeContext, type NativeContext } from '@/lib/native-chrome'

const HIDDEN: NativeContext = { percent: null, usedLabel: '', canCompact: false, compacting: false }

function usedLabel(telemetry: AgentTelemetry | null): string {
  const context = telemetry?.context
  if (context?.usedTokens === undefined || context.capacityTokens === undefined) return ''
  return `${context.usedTokens.toLocaleString('en-US')} of ${context.capacityTokens.toLocaleString('en-US')} tokens`
}

/**
 * Feeds the phone's native context meter from the visible chat, the one the composer gauge
 * reads on desktop, and runs its Compact. `onCompact` absent means compaction is not offered.
 */
export function useNativeContext({
  visible,
  telemetry,
  compacting,
  onCompact,
}: {
  visible: boolean
  telemetry: AgentTelemetry | null
  compacting: boolean
  onCompact: (() => void) | undefined
}) {
  const used = telemetry?.context?.usedPercent
  const snapshot = useMemo<NativeContext>(
    () =>
      !visible || used === undefined
        ? HIDDEN
        : {
            percent: Math.round(Math.max(0, Math.min(100, used))),
            usedLabel: usedLabel(telemetry),
            canCompact: onCompact !== undefined,
            compacting,
          },
    [visible, used, telemetry, onCompact, compacting],
  )
  useEffect(() => {
    setNativeContext(snapshot)
  }, [snapshot])
  useEffect(
    () => () => {
      setNativeContext(HIDDEN)
    },
    [],
  )

  // The latest gesture, without resubscribing on every render's new closure.
  const compact = useRef(onCompact)
  useEffect(() => {
    compact.current = onCompact
  }, [onCompact])
  useEffect(() => {
    if (!visible) return
    return onNativeChromeIntent((intent) => {
      if (intent.type === 'compact') compact.current?.()
    })
  }, [visible])
}

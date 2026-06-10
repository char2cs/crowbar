import { useEffect, useState } from 'react'
import { useConnectionStore } from '@/lib/ws/connection-store'

// Brief socket blips (e.g. a single channel cycling) should not flash the
// banner; only show it once the disconnect has persisted for a moment.
const SHOW_DELAY_MS = 500

/**
 * Slim, non-blocking pill shown while the backend is unreachable. Driven by
 * the wsManager's connection store: appears when every live channel is closed
 * or retrying, disappears as soon as any channel reconnects.
 */
export function ConnectionIndicator() {
  const status = useConnectionStore((state) => state.status)
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (status !== 'disconnected') {
      setVisible(false)
      return
    }
    const timer = setTimeout(() => setVisible(true), SHOW_DELAY_MS)
    return () => clearTimeout(timer)
  }, [status])

  if (!visible) return null

  return (
    <div
      role="status"
      data-testid="connection-indicator"
      className="fixed bottom-3 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-warning/40 bg-background px-3 py-1 text-[11px] text-warning shadow-md"
    >
      <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-warning" />
      <span>Backend unavailable — reconnecting…</span>
    </div>
  )
}

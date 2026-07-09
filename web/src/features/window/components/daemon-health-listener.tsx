import { useEffect } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { toast } from '@/features/window/stores/toast-store'

/**
 * Mount-once bridge from the desktop supervisor's daemon lifecycle events to
 * user-visible toasts. Without this, a daemon wedge or crash-restart cycle is
 * silent — the UI just goes quietly stale, which is exactly how the
 * 2026-07-04 daemon death went unnoticed for hours.
 *
 * `daemon:terminated` is deliberately not toasted: it fires on every exit,
 * including the intentional one during app shutdown, and every unexpected
 * exit is followed by either `daemon:restarted` or `daemon:dead`.
 */
export function DaemonHealthListener() {
  useEffect(() => {
    if (!isTauri()) return

    let disposed = false
    const unlistens: Array<() => void> = []

    void (async () => {
      const { listen } = await import('@tauri-apps/api/event')
      const subscriptions: Array<[string, () => void]> = [
        [
          'daemon:unhealthy',
          () =>
            toast.warning(
              'Backend unresponsive',
              'Capturing a diagnostics dump and restarting it…',
            ),
        ],
        [
          'daemon:restarted',
          () => toast.success('Backend restarted', 'Sessions reconnect automatically.'),
        ],
        [
          'daemon:dead',
          () =>
            toast.error(
              'Backend keeps failing',
              'Export diagnostics from Settings → Developer, then restart the app.',
            ),
        ],
      ]
      for (const [name, fire] of subscriptions) {
        const unlisten = await listen(name, fire)
        if (disposed) unlisten()
        else unlistens.push(unlisten)
      }
    })()

    return () => {
      disposed = true
      for (const unlisten of unlistens) unlisten()
    }
  }, [])

  return null
}

import { useEffect } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
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
 *
 * `daemon:halted` is a daemon that is alive but refused to open its home; its
 * payload is the reason. It goes to the halt store, which the root renders in
 * place of every route, and `daemon:restarted` clears it. A halt from before
 * this page mounted reaches the same store through the first refused request
 * or socket (lib/api/fetch, lib/ws/daemon-probe).
 */
export function DaemonHealthListener() {
  useEffect(() => {
    if (!isTauri()) return
    let disposed = false
    const unlistens: Array<() => void> = []

    void (async () => {
      const { listen } = await import('@tauri-apps/api/event')
      const subscriptions: Array<[string, (event: { payload: unknown }) => void]> = [
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
          () => {
            useDaemonHaltStore.getState().clear()
            toast.success('Backend restarted', 'Sessions reconnect automatically.')
          },
        ],
        [
          'daemon:dead',
          () =>
            toast.error(
              'Backend keeps failing',
              'Export diagnostics from Settings → Developer, then restart the app.',
            ),
        ],
        ['daemon:halted', ({ payload }) => useDaemonHaltStore.getState().halt(String(payload))],
      ]
      // Each subscription is an independent event name, so register them
      // concurrently instead of one at a time.
      const registered = await Promise.all(subscriptions.map(([name, fire]) => listen(name, fire)))
      for (const unlisten of registered) {
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

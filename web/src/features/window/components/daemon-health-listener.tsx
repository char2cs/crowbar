import { useEffect } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { toast } from '@/features/window/stores/toast-store'

// Surfaces the supervisor's daemon lifecycle so a wedge or crash loop isn't silent. Not
// `terminated` (also fires on intentional shutdown; restarted/dead follow a crash). `halted` goes
// to the halt store, cleared by `restarted`; an earlier halt arrives via the first refused request.
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

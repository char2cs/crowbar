import { useEffect } from 'react'
import { apiFetch } from '@/lib/api'
import { bestEffort } from '@/lib/best-effort'
import { isTauri } from '@/lib/crowbar-bridge'
import { toast } from '@/features/window/stores/toast-store'

interface DaemonHealth {
  status?: string
  detail?: { message?: string }
}

// One key, so the supervisor's event and the on-mount read collapse into one toast.
function showHalt(reason: unknown) {
  toast.show({
    key: 'daemon-halted',
    type: 'error',
    message: 'Crowbar could not start',
    description: String(reason),
    duration: 0,
  })
}

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
 * payload is the reason, and nothing restarts it, so the toast stays up. The
 * daemon halts before this page mounts, so mounting also asks it, the owner.
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
        ['daemon:halted', ({ payload }) => showHalt(payload)],
      ]
      // Each subscription is an independent event name, so register them
      // concurrently instead of one at a time.
      const registered = await Promise.all(subscriptions.map(([name, fire]) => listen(name, fire)))
      for (const unlisten of registered) {
        if (disposed) unlisten()
        else unlistens.push(unlisten)
      }
      if (disposed) return
      bestEffort(
        apiFetch<DaemonHealth | undefined>('/v0/health').then((health) => {
          if (!disposed && health?.status === 'halted') {
            showHalt(health.detail?.message ?? 'the daemon halted without saying why')
          }
        }),
        'daemon health read',
      )
    })()

    return () => {
      disposed = true
      for (const unlisten of unlistens) unlisten()
    }
  }, [])

  return null
}

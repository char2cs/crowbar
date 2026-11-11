import { useEffect } from 'react'
import { daemonCrashReport, isTauri } from '@/lib/crowbar-bridge'
import { useDaemonCrashStore } from '@/features/window/stores/daemon-crash-store'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { toast } from '@/features/window/stores/toast-store'
import { report as reportFailure } from '@/lib/errors/report'

const CRASH_CHECK = 'Check whether the backend is crash-looping'
const CRASH_CHECK_UNANSWERED = 'The desktop app did not answer; the backend may be fine.'

// Surfaces the supervisor's daemon lifecycle so a wedge or crash loop isn't silent. Not
// `terminated` (also fires on intentional shutdown). `halted` and `dead` go to their stores,
// cleared by `restarted`; a crash loop from before this window loaded is asked for on mount.
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
            useDaemonCrashStore.getState().clear()
            toast.success('Backend restarted', 'Sessions reconnect automatically.')
          },
        ],
        ['daemon:dead', ({ payload }) => useDaemonCrashStore.getState().crashed(String(payload))],
        ['daemon:halted', ({ payload }) => useDaemonHaltStore.getState().halt(String(payload))],
      ]
      // Each subscription is an independent event name, so register them
      // concurrently instead of one at a time.
      const registered = await Promise.all(subscriptions.map(([name, fire]) => listen(name, fire)))
      for (const unlisten of registered) {
        if (disposed) unlisten()
        else unlistens.push(unlisten)
      }
      // A failed ask says nothing about the daemon, so it is reported rather than shown as a crash.
      const report = await daemonCrashReport().catch((err: unknown) => {
        reportFailure(err, { action: CRASH_CHECK, fallback: CRASH_CHECK_UNANSWERED })
        return null
      })
      if (!disposed && report !== null) useDaemonCrashStore.getState().crashed(report)
    })()

    return () => {
      disposed = true
      for (const unlisten of unlistens) unlisten()
    }
  }, [])

  return null
}

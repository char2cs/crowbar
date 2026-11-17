import { useEffect, useRef } from 'react'
import { useConnectionStore, type Migrating } from '@/lib/ws/connection-store'
import { toast } from '@/features/window/stores/toast-store'

// One id for every warning/recovery toast, so disconnect cycles update one toast, not stack.
const TOAST_KEY = 'connection-indicator'

// Brief socket blips (e.g. a single channel cycling) should not flash the
// banner; only show it once the disconnect has persisted for a moment.
const SHOW_DELAY_MS = 500

// Without a step the daemon is only starting (every boot passes through
// migrating), so its own message is the title, never "upgrading its data".
function migratingToast(migrating: Migrating) {
  const { step } = migrating
  return {
    type: 'info' as const,
    message: step ? 'Crowbar is upgrading its data' : migrating.message,
    description: step ? `Step ${step.index} of ${step.total}: ${step.name}` : undefined,
    key: TOAST_KEY,
    duration: 0,
  }
}

// Drives a toast for sustained backend outages from the connection store (toasts come from
// components, never stores). The outage is a STATE, so its toast has `duration: 0` (no auto-close)
// and is dismissed only here, when a channel reconnects.
export function ConnectionIndicator() {
  const status = useConnectionStore((state) => state.status)
  const migrating = useConnectionStore((state) => state.migrating)
  const shownRef = useRef(false)

  useEffect(() => {
    // A migrating daemon is shown at once and updated in place as steps advance.
    if (status === 'migrating' && migrating) {
      shownRef.current = true
      toast.show(migratingToast(migrating))
      return
    }
    if (status !== 'disconnected') {
      if (shownRef.current) {
        shownRef.current = false
        toast.dismissByKey(TOAST_KEY)
        // Confirm recovery only when the warning was shown; the shared key stops a flappy reconnect
        // stacking confirmations.
        toast.show({
          message: 'Reconnected',
          description: 'Backend connection restored.',
          type: 'success',
          key: TOAST_KEY,
        })
      }
      return
    }

    const timer = setTimeout(() => {
      shownRef.current = true
      toast.show({
        message: 'Backend unavailable',
        description: 'Reconnecting…',
        type: 'warning',
        key: TOAST_KEY,
        duration: 0,
      })
    }, SHOW_DELAY_MS)

    return () => clearTimeout(timer)
  }, [status, migrating])

  return null
}

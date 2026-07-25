import { useEffect, useRef } from 'react'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { toast } from '@/features/window/stores/toast-store'

// Fixed dedup key: every warning/recovery toast for this component reuses it,
// so repeated disconnect/reconnect cycles update or replace the same toast
// instead of stacking duplicates (toastManager.add() with a matching id
// updates the existing toast rather than adding a new one).
const TOAST_KEY = 'connection-indicator'

// Brief socket blips (e.g. a single channel cycling) should not flash the
// banner; only show it once the disconnect has persisted for a moment.
const SHOW_DELAY_MS = 500

/**
 * Effect-only component: watches the wsManager's connection store and drives
 * a toast for sustained backend outages. Per CLAUDE.md, a toast is fired from
 * a component watching store state, never from a store or the backend itself
 * — see PlaceholderToastWatcher for the sibling pattern this follows.
 *
 * "Backend unavailable" is a persistent STATE, not a one-off event: a toast
 * that auto-dismissed after a few seconds would silently remove the only
 * indication that the app is disconnected while it's still disconnected.
 * So the warning toast is shown with `duration: 0` — ToastStore.addToast
 * (@base-ui/react/toast) only schedules an auto-close timer when the
 * resolved duration is `> 0`, so `0` opts a toast out of it entirely — and it
 * is dismissed only here, the instant a channel reconnects, never by a timer.
 */
export function ConnectionIndicator() {
  const status = useConnectionStore((state) => state.status)
  const shownRef = useRef(false)

  useEffect(() => {
    if (status !== 'disconnected') {
      if (shownRef.current) {
        shownRef.current = false
        toast.dismissByKey(TOAST_KEY)
        // A brief, auto-dismissing confirmation that the outage is over —
        // only fired when the outage was actually surfaced (shownRef), so a
        // blip that never showed the warning doesn't get a pointless
        // "Reconnected" afterward. Reuses the same key so a flappy
        // reconnect can't stack up several confirmations either.
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
  }, [status])

  return null
}

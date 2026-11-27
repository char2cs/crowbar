import { useEffect, useRef } from 'react'
import { useConnectionStore, type Migrating } from '@/lib/ws/connection-store'
import { toast } from '@/features/window/stores/toast-store'

const TOAST_KEY = 'connection-indicator'

const SHOW_DELAY_MS = 500

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

export function ConnectionIndicator() {
  const status = useConnectionStore((state) => state.status)
  const migrating = useConnectionStore((state) => state.migrating)
  const shownRef = useRef(false)

  useEffect(() => {
    if (status === 'migrating' && migrating) {
      shownRef.current = true
      toast.show(migratingToast(migrating))
      return
    }
    if (status !== 'disconnected') {
      if (shownRef.current) {
        shownRef.current = false
        toast.dismissByKey(TOAST_KEY)
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

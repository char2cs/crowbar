import { useCallback, useEffect, useRef, useState } from 'react'
import { report } from '@/lib/errors/report'
import { browserApi } from '@/lib/browser-bridge'
import { toast } from '@/features/window/stores/toast-store'
import { cancelPick, formatPointer, pickPointer } from '@/features/browser/lib/pointer'

/**
 * Pick mode for one tab. The page owns the overlay and the pending pick; this
 * only starts it, writes the result to the clipboard, and cancels it when the
 * tab goes off screen or unmounts.
 */
export function usePointerPick(bufferId: string, visible: boolean) {
  const [picking, setPicking] = useState(false)
  const inFlight = useRef(false)

  const stop = useCallback(() => {
    if (inFlight.current)
      void cancelPick(bufferId).catch((err: unknown) => report(err, { action: 'Cancel the pick' }))
  }, [bufferId])

  const start = useCallback(async () => {
    inFlight.current = true
    setPicking(true)
    try {
      const result = await pickPointer(bufferId)
      if (!result) return
      await browserApi.copyText(formatPointer(result, bufferId))
      const count = result.items.length + result.more
      toast.success('Pointer copied', count > 1 ? `${count} items` : undefined)
    } catch (err) {
      // The native side rejects with its own sentence, not an Error.
      report(err, { action: 'Could not pick', fallback: typeof err === 'string' ? err : undefined })
    } finally {
      inFlight.current = false
      setPicking(false)
    }
  }, [bufferId])

  const toggle = useCallback(() => {
    if (inFlight.current) stop()
    else void start()
  }, [start, stop])

  useEffect(() => {
    if (!picking || !visible) return
    return stop
  }, [picking, visible, stop])

  return { picking, toggle }
}

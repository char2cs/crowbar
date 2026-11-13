import { useEffect, useEffectEvent, type RefObject } from 'react'
import {
  onNativeChromeIntent,
  setNativeAttachSheet,
  type NativeAttachSheet,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

interface NativeAttachSheetProps {
  open: boolean
  sheet: Omit<NativeAttachSheet, 'open'>
  onIntent: (intent: NativeChromeIntent) => void
}

/** The Attach sheet as the phone's native sheet: the whole view-model is pushed on every change,
 *  and taps come back as intents for the web, which owns every upload. */
export function NativeAttachSheetView({ open, sheet, onIntent }: NativeAttachSheetProps) {
  // Keyed by the content, not the object's identity: a render that changed nothing pushes nothing.
  const key = JSON.stringify(sheet)
  useEffect(() => {
    if (open) setNativeAttachSheet({ open: true, ...(JSON.parse(key) as typeof sheet) })
  }, [open, key])
  // The closing push repeats the last content, so the sheet animates away still showing it.
  const close = useEffectEvent(() =>
    setNativeAttachSheet({ open: false, ...(JSON.parse(key) as typeof sheet) }),
  )
  useEffect(() => {
    if (!open) return
    return close
  }, [open])

  const handle = useEffectEvent(onIntent)
  useEffect(() => {
    if (!open) return
    return onNativeChromeIntent(handle)
  }, [open])
  return null
}

/** A dismissed system picker reports `cancel`, not `change`. */
export function useOnCancel(
  ref: RefObject<HTMLInputElement | null>,
  active: boolean,
  handler: () => void,
) {
  const onCancel = useEffectEvent(handler)
  useEffect(() => {
    const input = ref.current
    if (!active || !input) return
    input.addEventListener('cancel', onCancel)
    return () => input.removeEventListener('cancel', onCancel)
  }, [ref, active])
}

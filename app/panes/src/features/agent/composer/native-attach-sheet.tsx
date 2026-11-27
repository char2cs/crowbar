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

export function NativeAttachSheetView({ open, sheet, onIntent }: NativeAttachSheetProps) {
  const key = JSON.stringify(sheet)
  useEffect(() => {
    if (open) setNativeAttachSheet({ open: true, ...(JSON.parse(key) as typeof sheet) })
  }, [open, key])
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

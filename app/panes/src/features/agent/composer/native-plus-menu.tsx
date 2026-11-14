import { useEffect, useEffectEvent } from 'react'
import {
  onNativeChromeIntent,
  setNativePlusMenu,
  type NativeChromeIntent,
  type NativePlusMenu,
} from '@/lib/native-chrome'

type NativePlusItem = NativePlusMenu['items'][number]

interface NativePlusMenuProps {
  open: boolean
  title: string
  items: NativePlusItem[]
  onPick: (id: string) => void
  onClose: () => void
}

/** The plus menu as the phone's native sheet: pushed whole while open, a row answers with its id. */
export function NativePlusMenuSheet({ open, title, items, onPick, onClose }: NativePlusMenuProps) {
  // Keyed by the rows themselves, not the array's identity.
  const key = JSON.stringify(items)
  useEffect(() => {
    if (!open) return
    setNativePlusMenu({ open, title, items: JSON.parse(key) as NativePlusItem[] })
    return () => setNativePlusMenu({ open: false, title, items: [] })
  }, [open, title, key])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'pickPlusItem') onPick(intent.id)
    if (intent.type === 'closePlusMenu') onClose()
  })
  useEffect(() => {
    if (!open) return
    return onNativeChromeIntent(onIntent)
  }, [open])
  return null
}

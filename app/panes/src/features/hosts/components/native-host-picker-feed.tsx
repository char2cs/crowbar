import { useEffect, useEffectEvent } from 'react'
import { useNativeHostsSnapshot } from '@/features/hosts/hooks/use-native-hosts-snapshot'
import { useUIState } from '@/features/window/stores/ui-state-store'
import {
  onNativeChromeIntent,
  setNativeHostPicker,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

const close = () => useUIState.getState().setIsHostPickerOpen(false)

/**
 * Feeds the phone's native host picker sheet while it is open: the paired hosts, pushed whole on
 * each change. Picking a host is the chat list's own `selectHost` intent; the sheet closes with
 * it. Revoke asks natively first; Add host is the sidebar's intent.
 */
export function NativeHostPickerFeed({ shownHost }: { shownHost: string | null }) {
  const { snapshot, revoke } = useNativeHostsSnapshot(shownHost)

  useEffect(() => {
    setNativeHostPicker({ ...snapshot, open: true })
  }, [snapshot])
  // Unmounting is how the web closes the sheet.
  useEffect(
    () => () => setNativeHostPicker({ hosts: [], revoking: [], shownHost: null, open: false }),
    [],
  )

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'revokeHost') revoke(intent.id)
    if (intent.type === 'selectHost' || intent.type === 'closeHostPicker') close()
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])
  return null
}

import { useEffect, useEffectEvent } from 'react'
import { useNativeHostsSnapshot } from '@/features/hosts/hooks/use-native-hosts-snapshot'
import { useUIState } from '@/features/window/stores/ui-state-store'
import {
  onNativeChromeIntent,
  setNativeHostPicker,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

const close = () => useUIState.getState().setIsHostPickerOpen(false)

export function NativeHostPickerFeed({ shownHost }: { shownHost: string | null }) {
  const { snapshot, revoke } = useNativeHostsSnapshot(shownHost)

  useEffect(() => {
    setNativeHostPicker({ ...snapshot, open: true })
  }, [snapshot])
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

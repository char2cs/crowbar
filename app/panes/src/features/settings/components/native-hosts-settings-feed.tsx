import { useEffect, useEffectEvent } from 'react'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useNativeHostsSnapshot } from '@/features/hosts/hooks/use-native-hosts-snapshot'
import {
  onNativeChromeIntent,
  setNativeHostsSettings,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

export function NativeHostsSettingsFeed({ shownHost }: { shownHost: string | null }) {
  const { snapshot, revoke } = useNativeHostsSnapshot(shownHost)

  useEffect(() => {
    setNativeHostsSettings(snapshot)
  }, [snapshot])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'revokeHost' && !useUIState.getState().isHostPickerOpen) revoke(intent.id)
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])
  return null
}

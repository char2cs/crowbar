import { useEffect, useEffectEvent } from 'react'
import { useNativeHostsSnapshot } from '@/features/hosts/hooks/use-native-hosts-snapshot'
import {
  onNativeChromeIntent,
  setNativeHostsSettings,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

/**
 * Feeds the phone's Hosts settings tab (a webview with no IPC) while the native settings sheet is
 * up: every paired host, pushed whole on each change. A host picked to show is the chat list's
 * own `selectHost` intent.
 */
export function NativeHostsSettingsFeed({ shownHost }: { shownHost: string | null }) {
  const { snapshot, revoke } = useNativeHostsSnapshot(shownHost)

  useEffect(() => {
    setNativeHostsSettings(snapshot)
  }, [snapshot])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    if (intent.type === 'revokeHost') revoke(intent.id)
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])
  return null
}

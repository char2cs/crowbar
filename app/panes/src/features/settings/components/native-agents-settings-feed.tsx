import { useEffect, useEffectEvent } from 'react'
import { useDefaultPermissionLevel } from '@/features/settings/components/tabs/default-permission-level-setting'
import { useProvidersSettings } from '@/features/settings/hooks/use-providers-settings'
import {
  onNativeChromeIntent,
  setNativeAgentsSettings,
  type NativeChromeIntent,
} from '@/lib/native-chrome'

/** Feeds the phone's Agents settings page (a webview with no IPC) while the native sheet is up:
 *  `hostId`'s agents via the desktop tab's hooks, pushed whole on change; edits use its writes. */
export function NativeAgentsSettingsFeed({
  hostId,
  hostName,
}: {
  hostId: string
  hostName: string
}) {
  const { providers, status, toggle, toggleTools, reorder } = useProvidersSettings(hostId)
  const { level, change } = useDefaultPermissionLevel(hostId)

  useEffect(() => {
    setNativeAgentsSettings({ hostId, hostName, status, providers, permissionLevel: level })
  }, [hostId, hostName, status, providers, level])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    switch (intent.type) {
      case 'toggleProvider':
        toggle(intent.id, intent.enabled)
        return
      case 'toggleProviderTools':
        toggleTools(intent.id, intent.enabled)
        return
      case 'reorderProviders':
        reorder(intent.ids)
        return
      case 'setDefaultPermission':
        change(intent.level)
        return
    }
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])
  return null
}

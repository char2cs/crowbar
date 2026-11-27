import type { PaneClient } from '@/sdk/pane-client'
import { routeSettingsToShell } from '@/features/settings/lib/shell-settings'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { useSettingsStore } from '@/features/settings/store'

export function followShellSettings(
  client: Pick<PaneClient, 'onSettings' | 'setSetting'>,
): () => void {
  const stopWrites = routeSettingsToShell((key, value) => client.setSetting(key, value))
  const stopValues = client.onSettings((values) => {
    useSettingsStore.getState().applyShellSettings(values)
    useKeymapStore.getState().actions.adoptShellOverrides(values.keybindingUserOverrides)
  })
  return () => {
    stopWrites()
    stopValues()
  }
}

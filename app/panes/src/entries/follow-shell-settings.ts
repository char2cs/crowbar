import type { PaneClient } from '@/sdk/pane-client'
import { routeSettingsToShell } from '@/features/settings/lib/shell-settings'
import { useSettingsStore } from '@/features/settings/store'

/** Makes the shell's settings the pane's settings on mount and on every change, and sends the pane's own writes to the shell; returns the stop. */
export function followShellSettings(
  client: Pick<PaneClient, 'onSettings' | 'setSetting'>,
): () => void {
  const stopWrites = routeSettingsToShell((key, value) => client.setSetting(key, value))
  const stopValues = client.onSettings((values) =>
    useSettingsStore.getState().applyShellSettings(values),
  )
  return () => {
    stopWrites()
    stopValues()
  }
}

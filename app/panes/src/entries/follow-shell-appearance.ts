import type { PaneClient } from '@/sdk/pane-client'
import type { ThemeTokens } from '@/sdk/theme-tokens'
import { useSettingsStore } from '@/features/settings/store'

const THEME_ID_TOKEN = 'theme-id'
const THEME_TYPE_TOKEN = 'theme-type'

function applyTokens(tokens: ThemeTokens): void {
  const { updateSetting } = useSettingsStore.getState()
  const themeId = tokens[THEME_ID_TOKEN]
  const themeType = tokens[THEME_TYPE_TOKEN]
  if (themeId) void updateSetting('theme', themeId)
  if (themeType === 'light' || themeType === 'dark') void updateSetting('themeMode', themeType)
}

/** Makes the shell's `theme-id` and `theme-type` tokens the pane's `theme` and `themeMode` settings, so the pane looks like the shell around it; returns the stop. */
export function followShellAppearance(client: Pick<PaneClient, 'onTheme'>): () => void {
  return client.onTheme(applyTokens)
}

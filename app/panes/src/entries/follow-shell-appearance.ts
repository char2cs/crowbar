import type { PaneClient } from '@/sdk/pane-client'
import type { ThemeTokens } from '@/sdk/theme-tokens'
import { useSettingsStore } from '@/features/settings/store'

const THEME_ID_TOKEN = 'theme-id'
const THEME_TYPE_TOKEN = 'theme-type'

function applyTokens(tokens: ThemeTokens): void {
  const { updateSetting } = useSettingsStore.getState()
  const themeId = tokens[THEME_ID_TOKEN]
  const themeType = tokens[THEME_TYPE_TOKEN]
  if (themeType === 'light' || themeType === 'dark') void updateSetting('themeMode', themeType)
  if (themeId) void updateSetting('theme', themeId)
}

/** Makes the shell's `theme-id` and `theme-type` tokens the pane's `theme` and `themeMode` settings, so the pane looks like the shell around it; returns the stop. The mode is written before the theme because setting the theme re-applies the stored mode, and `system` would briefly follow the webview's own appearance instead of the shell's. */
export function followShellAppearance(client: Pick<PaneClient, 'onTheme'>): () => void {
  return client.onTheme(applyTokens)
}

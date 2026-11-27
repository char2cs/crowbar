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

export function followShellAppearance(client: Pick<PaneClient, 'onTheme'>): () => void {
  return client.onTheme(applyTokens)
}

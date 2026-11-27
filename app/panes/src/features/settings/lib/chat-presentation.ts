import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'

export type ChatPresentation = 'chat' | 'terminal' | 'split'

export type LandingChatPresentation = Exclude<ChatPresentation, 'split'>

function selectDefaultChatPresentation(state: {
  settings: Pick<Settings, 'chatIsDefaultPresentation'>
}): LandingChatPresentation {
  return state.settings.chatIsDefaultPresentation ? 'chat' : 'terminal'
}

export function getDefaultChatPresentation(): LandingChatPresentation {
  return selectDefaultChatPresentation(useSettingsStore.getState())
}

const SPLIT_PRESENTATION_AVAILABLE: boolean = import.meta.env.DEV

export function useSplitPresentationEnabled(): boolean {
  const enabled = useSettingsStore((s) => s.settings.chatSplitPresentationEnabled)
  return SPLIT_PRESENTATION_AVAILABLE && enabled
}

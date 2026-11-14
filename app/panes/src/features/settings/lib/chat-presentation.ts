import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'

// `split` is a diagnostic, not a product surface: the reconstructed chat beside the CLI's own TUI,
// so a divergence shows by comparison. Dev builds only, behind the Developer tab's switch.
export type ChatPresentation = 'chat' | 'terminal' | 'split'

// Split is an instrument the user reaches for, never a place a chat opens by itself.
export type LandingChatPresentation = Exclude<ChatPresentation, 'split'>

// Which surface a chat lands on, never which exist: only the seed of local state. User-level, in
// the global settings store, since per-workspace stores are destroyed on eviction.
export function selectDefaultChatPresentation(state: {
  settings: Pick<Settings, 'chatIsDefaultPresentation'>
}): LandingChatPresentation {
  return state.settings.chatIsDefaultPresentation ? 'chat' : 'terminal'
}

// Deliberately not a subscribing hook: an open chat keeps its surface when the preference
// changes. Pass straight to a `useState` initializer.
export function getDefaultChatPresentation(): LandingChatPresentation {
  return selectDefaultChatPresentation(useSettingsStore.getState())
}

// Build-time half of the split gate (folded away in production, like the Developer tab); the
// stored preference is the run-time half, and both must say yes.
export const SPLIT_PRESENTATION_AVAILABLE: boolean = import.meta.env.DEV

// Subscribing, unlike the landing seed: toggling must add or remove the Split button live, and a
// chat sitting in split falls back to Chat (agent-chat-pane derives its surface from this).
export function useSplitPresentationEnabled(): boolean {
  const enabled = useSettingsStore((s) => s.settings.chatSplitPresentationEnabled)
  return SPLIT_PRESENTATION_AVAILABLE && enabled
}

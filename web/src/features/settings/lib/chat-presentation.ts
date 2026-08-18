import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'

/** The two surfaces a chat can be shown on. */
export type ChatPresentation = 'chat' | 'terminal'

/**
 * Which surface a chat LANDS on. Never which surfaces exist: both stay reachable
 * from the chat's own switcher whichever way the preference sits, so this is
 * only ever the starting value of a piece of local state.
 *
 * The preference is user-level and lives in the global settings store, not in the
 * per-workspace registry — that registry is destroyed on a workspace switch, and
 * a landing surface the user chose once must outlive one.
 */
export function selectDefaultChatPresentation(state: {
  settings: Pick<Settings, 'chatIsDefaultPresentation'>
}): ChatPresentation {
  return state.settings.chatIsDefaultPresentation ? 'chat' : 'terminal'
}

/**
 * THE READ, and deliberately not a subscribing hook. This value is only ever the
 * SEED of a surface the user then drives by hand, so a chat already open must
 * keep the surface it is on when the preference changes underneath it — pass
 * this straight to a `useState` initializer.
 */
export function getDefaultChatPresentation(): ChatPresentation {
  return selectDefaultChatPresentation(useSettingsStore.getState())
}

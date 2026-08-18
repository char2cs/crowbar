import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'

/**
 * The surfaces a chat can be shown on.
 *
 * `split` is a DIAGNOSTIC, not a product surface: it puts the chat Crowbar
 * reconstructed from hooks next to the CLI's own TUI — the ground truth — so a
 * turn that never closed, a message that never landed or a prompt the chat could
 * not see is visible by comparison instead of by bisection. It is reachable only
 * in a development build, and only once the Developer tab's switch is on.
 */
export type ChatPresentation = 'chat' | 'terminal' | 'split'

/**
 * The surfaces a chat can LAND on. Split is deliberately not one of them: it is
 * an instrument the user reaches for, never a place a chat opens by itself.
 */
export type LandingChatPresentation = Exclude<ChatPresentation, 'split'>

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
}): LandingChatPresentation {
  return state.settings.chatIsDefaultPresentation ? 'chat' : 'terminal'
}

/**
 * THE READ, and deliberately not a subscribing hook. This value is only ever the
 * SEED of a surface the user then drives by hand, so a chat already open must
 * keep the surface it is on when the preference changes underneath it — pass
 * this straight to a `useState` initializer.
 */
export function getDefaultChatPresentation(): LandingChatPresentation {
  return selectDefaultChatPresentation(useSettingsStore.getState())
}

/**
 * Whether this BUILD can offer the split view at all — the build-time half of
 * the gate, folded away by the bundler in production exactly like the Developer
 * tab that carries the switch. The stored preference is the run-time half, and
 * both have to say yes.
 *
 * Read as a module constant rather than inline so the two halves are named in
 * one place and a test can assert the pairing rather than a boolean literal.
 */
export const SPLIT_PRESENTATION_AVAILABLE: boolean = import.meta.env.DEV

/**
 * Is Split offered in this chat's surface switcher?
 *
 * A SUBSCRIBING read, unlike the landing preference above, and for the opposite
 * reason: this one is about what EXISTS, not about where a chat starts. Turning
 * the switch on has to put the third button in front of the user without a
 * reload, and turning it off has to take it away again — including from a chat
 * that is sitting in split right now (agent-chat-pane derives its surface
 * through this, so it falls back to Chat rather than stranding anyone on a
 * surface they can no longer leave).
 */
export function useSplitPresentationEnabled(): boolean {
  const enabled = useSettingsStore((s) => s.settings.chatSplitPresentationEnabled)
  return SPLIT_PRESENTATION_AVAILABLE && enabled
}

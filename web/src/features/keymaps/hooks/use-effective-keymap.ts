/**
 * Hooks for reading effective keybindings from the keymap store.
 *
 * Components/hooks subscribe to the active preset + overrides and receive a
 * memoized commandId -> chord map (or a single chord). Because they subscribe
 * narrowly, changing the preset or an override re-resolves bindings live.
 */
import { useMemo } from 'react'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { getEffectiveChordMap } from '@/features/keymaps/utils/effective-keymaps'

/** Full commandId -> effective chord map for all commands. */
export function useEffectiveChordMap(): Record<string, string> {
  const preset = useKeymapStore.use.preset()
  const overrides = useKeymapStore.use.overrides()
  return useMemo(() => getEffectiveChordMap({ preset, overrides }), [preset, overrides])
}

/** Effective chord for a single command id. */
export function useEffectiveKeybinding(commandId: string): string | undefined {
  const map = useEffectiveChordMap()
  return map[commandId]
}

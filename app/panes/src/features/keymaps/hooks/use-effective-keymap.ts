// Subscribe narrowly to the active preset + overrides and memoize the resolved chords, so a
// preset or override change re-resolves bindings live.
import { useMemo } from 'react'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { getEffectiveChordMap } from '@/features/keymaps/utils/effective-keymaps'

/** Full commandId -> effective chord map for all commands. */
export function useEffectiveChordMap(): Record<string, string> {
  const preset = useKeymapStore.use.preset()
  const overrides = useKeymapStore.use.overrides()
  return useMemo(() => getEffectiveChordMap({ preset, overrides }), [preset, overrides])
}

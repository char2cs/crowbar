import { useMemo } from 'react'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { getEffectiveChordMap } from '@/features/keymaps/utils/effective-keymaps'

export function useEffectiveChordMap(): Record<string, string> {
  const preset = useKeymapStore.use.preset()
  const overrides = useKeymapStore.use.overrides()
  return useMemo(() => getEffectiveChordMap({ preset, overrides }), [preset, overrides])
}

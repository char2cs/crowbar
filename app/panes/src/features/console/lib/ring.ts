/** Appends `add` to `list`, keeping only the newest `cap` items. Never mutates its inputs. */
export function appendCapped<T>(list: readonly T[], add: readonly T[], cap: number): T[] {
  if (add.length === 0) return list.length <= cap ? [...list] : list.slice(list.length - cap)
  const merged = list.length === 0 ? [...add] : [...list, ...add]
  return merged.length <= cap ? merged : merged.slice(merged.length - cap)
}

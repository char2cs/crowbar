/** Place draggedId immediately before targetId in the full ordered id list. */
export function reorderIds(orderedIds: string[], draggedId: string, targetId: string): string[] {
  if (draggedId === targetId) return orderedIds
  const without = orderedIds.filter((id) => id !== draggedId)
  const idx = without.indexOf(targetId)
  if (idx === -1) return orderedIds
  return [...without.slice(0, idx), draggedId, ...without.slice(idx)]
}

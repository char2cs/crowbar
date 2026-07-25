/**
 * Move `draggedId` into drop SLOT `slot` — the gap above row `slot`, with
 * `orderedIds.length` meaning the end of the list (agent-chat-drop-geometry.ts
 * resolves the same slots from pointer geometry, and the panel draws a line in
 * them).
 *
 * Slots are indexed against the list AS THE USER SEES IT — the dragged row still
 * in place — so the two slots touching that row are no-ops (dropping a row just
 * above or just below itself leaves it where it is). Every other slot moves it,
 * including the last one; expressing the drop as "insert before row X" could not
 * name the end of the list at all.
 */
export function reorderIds(orderedIds: string[], draggedId: string, slot: number): string[] {
  const from = orderedIds.indexOf(draggedId)
  if (from === -1) return orderedIds
  const target = Math.max(0, Math.min(slot, orderedIds.length))
  if (target === from || target === from + 1) return orderedIds
  const without = orderedIds.filter((id) => id !== draggedId)
  // Removing the dragged row shifts every later slot up by one.
  const at = target > from ? target - 1 : target
  return [...without.slice(0, at), draggedId, ...without.slice(at)]
}

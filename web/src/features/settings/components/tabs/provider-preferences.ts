import { arrayMove } from '@dnd-kit/sortable'
import type { AgentProvider, ProviderPreference } from '@/features/agent/api/agent-api'

// Pure mappings behind the Providers settings tab. The tab reads its live list
// from the active workspace store and, on any change, PUTs the COMPLETE ordered
// preference set (spec §3.2) — so both a reorder and a toggle funnel through the
// same "ordered ids + per-id disabled flags → payload" shape. Factored out here
// (rather than driving a real @dnd-kit pointer drag, which is brittle in jsdom)
// so the reorder→payload and toggle→payload mappings are unit-testable directly,
// mirroring how the tab bar's `reorderIds` helper is tested.

/** The disabled-by-id map for the current provider list (disabled = !enabled) —
 *  the baseline a toggle flips one entry of and a reorder carries unchanged. */
export function providerDisabledMap(providers: AgentProvider[]): Record<string, boolean> {
  const map: Record<string, boolean> = {}
  for (const p of providers) map[p.id] = !p.enabled
  return map
}

/** Build the COMPLETE ordered preference set the backend replaces its table with:
 *  array index becomes the new priority; `disabled` comes from the toggle map
 *  (an id absent from the map defaults to enabled — spec §3.1). */
export function buildProviderPreferences(
  orderedIds: string[],
  disabledById: Record<string, boolean>,
): ProviderPreference[] {
  return orderedIds.map((id) => ({ id, disabled: disabledById[id] ?? false }))
}

/** Apply the SAME intent the payload carries to the live provider list — the
 *  optimistic half of a write, so a toggle moves on the click rather than on the
 *  response (`Switch checked={provider.enabled}` is fully controlled off this
 *  list, so without it the switch visibly refuses to move for a whole round trip
 *  — and a second toggle taken in that window is built from the pre-write
 *  snapshot, which UNDOES the first). Ids the list doesn't have are skipped;
 *  every other field of a row is carried through untouched. */
export function applyProviderPreferences(
  providers: AgentProvider[],
  orderedIds: string[],
  disabledById: Record<string, boolean>,
): AgentProvider[] {
  const byId = new Map(providers.map((p) => [p.id, p]))
  return orderedIds.flatMap((id) => {
    const provider = byId.get(id)
    return provider ? [{ ...provider, enabled: !(disabledById[id] ?? false) }] : []
  })
}

/** Move `activeId` into `overId`'s slot — the reorder a dnd-kit `onDragEnd`
 *  performs, expressed as a pure id transform (no-op when the drag didn't move,
 *  or either id is unknown). */
export function reorderProviderIds(
  orderedIds: string[],
  activeId: string,
  overId: string,
): string[] {
  const from = orderedIds.indexOf(activeId)
  const to = orderedIds.indexOf(overId)
  if (from === -1 || to === -1 || from === to) return orderedIds
  return arrayMove(orderedIds, from, to)
}

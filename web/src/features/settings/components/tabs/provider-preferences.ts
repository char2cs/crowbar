import { arrayMove } from '@dnd-kit/sortable'
import type { AgentProvider, ProviderPreference } from '@/features/agent/api/agent-api'

// Pure mappings behind the Providers settings tab. The tab reads its live list
// from the active workspace store and, on any change, PUTs the COMPLETE ordered
// preference set (spec §3.2) — so both a reorder and a toggle funnel through the
// same "ordered ids + per-id flags → payload" shape. Factored out here (rather
// than driving a real @dnd-kit pointer drag, which is brittle in jsdom) so the
// reorder→payload and toggle→payload mappings are unit-testable directly,
// mirroring how the tab bar's `reorderIds` helper is tested.

/** The two per-provider switches, in the NEGATIVE polarity the backend stores and
 *  the payload carries: `disabled` is the provider itself, `mcpDisabled` is
 *  Crowbar's tool surface inside it. They are independent axes — a provider can be
 *  offered with its tools off, and an unavailable provider keeps whatever tool
 *  setting it had. */
export interface ProviderFlags {
  disabled: boolean
  mcpDisabled: boolean
}

/** The flags-by-id map for the current provider list (each flag = !its positive
 *  reading) — the baseline a toggle flips ONE FIELD of one entry of, and a reorder
 *  carries through unchanged.
 *
 *  It carries BOTH flags because the write it feeds replaces whole rows: a map of
 *  `disabled` alone would leave every reorder and every enable/disable PUTting a
 *  zero `mcpDisabled`, quietly switching the tool surface back on behind the user. */
export function providerDisabledMap(providers: AgentProvider[]): Record<string, ProviderFlags> {
  const map: Record<string, ProviderFlags> = {}
  for (const p of providers) map[p.id] = { disabled: !p.enabled, mcpDisabled: !p.mcpEnabled }
  return map
}

/** Build the COMPLETE ordered preference set the backend replaces its table with:
 *  array index becomes the new priority; both flags come from the flags map (an id
 *  absent from the map defaults to enabled, with its tools on — spec §3.1). */
export function buildProviderPreferences(
  orderedIds: string[],
  flagsById: Record<string, ProviderFlags>,
): ProviderPreference[] {
  return orderedIds.map((id) => ({
    id,
    disabled: flagsById[id]?.disabled ?? false,
    mcpDisabled: flagsById[id]?.mcpDisabled ?? false,
  }))
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
  flagsById: Record<string, ProviderFlags>,
): AgentProvider[] {
  const byId = new Map(providers.map((p) => [p.id, p]))
  return orderedIds.flatMap((id) => {
    const provider = byId.get(id)
    if (!provider) return []
    return [
      {
        ...provider,
        enabled: !(flagsById[id]?.disabled ?? false),
        mcpEnabled: !(flagsById[id]?.mcpDisabled ?? false),
      },
    ]
  })
}

/** Move `activeId` into `overId`'s slot — the reorder a dnd-kit `onDragEnd`
 *  performs, expressed as a pure id transform (no-op when the drag didn't move,
 *  or either id is unknown).
 *
 *  Ids only: a drag changes priority and nothing else, and the flags ride to the
 *  payload untouched through the map `buildProviderPreferences` is given. */
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

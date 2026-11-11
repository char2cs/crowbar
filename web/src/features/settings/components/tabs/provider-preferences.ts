import { arrayMove } from '@dnd-kit/sortable'
import type { AgentProvider, ProviderPreference } from '@/features/agent/api/agent-providers-api'

// Every Providers change PUTs the complete ordered preference set, so reorders and toggles share
// one "ordered ids + per-id flags -> payload" shape, kept pure (a jsdom dnd-kit drag is brittle).

/** Negative polarity, as the backend stores it. Independent axes: `disabled` is the provider,
 *  `mcpDisabled` Crowbar's tool surface inside it. */
export interface ProviderFlags {
  disabled: boolean
  mcpDisabled: boolean
}

/** The baseline a toggle flips one field of. Both flags, because the write replaces whole rows: a
 *  `disabled`-only map would PUT a zero `mcpDisabled` and silently re-enable tools. */
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

/** The optimistic half of a write: the controlled switches move on click, and a second toggle
 *  mid-round-trip builds on this, not the pre-write list (which would undo the first). Unknown
 *  ids are skipped; other row fields pass through. */
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

/** Move `activeId` into `overId`'s slot (a dnd-kit `onDragEnd`); no-op if unmoved or unknown.
 *  Ids only: the flags ride to the payload untouched. */
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

import { arrayMove } from '@dnd-kit/sortable'
import type { AgentProvider, ProviderPreference } from '@/features/agent/api/agent-providers-api'

export interface ProviderFlags {
  disabled: boolean
  mcpDisabled: boolean
}

export function providerDisabledMap(providers: AgentProvider[]): Record<string, ProviderFlags> {
  const map: Record<string, ProviderFlags> = {}
  for (const p of providers) map[p.id] = { disabled: !p.enabled, mcpDisabled: !p.mcpEnabled }
  return map
}

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

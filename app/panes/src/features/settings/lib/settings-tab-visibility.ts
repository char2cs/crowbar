import type { SettingsTab } from '@/features/window/stores/ui-state-store'

/** A null `matchingTabs` means no search is active, so every tab stays visible. */
export function filterSettingsTabsBySearch<T extends { id: SettingsTab }>(
  tabs: T[],
  matchingTabs: Set<SettingsTab> | null | undefined,
): T[] {
  if (!matchingTabs) {
    return tabs
  }

  return tabs.filter((item) => matchingTabs.has(item.id))
}

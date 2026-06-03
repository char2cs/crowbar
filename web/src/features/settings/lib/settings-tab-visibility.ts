import type { SettingsTab } from "@/features/window/stores/ui-state-store";

export function useSettingsTabsFiltered<T extends { id: SettingsTab }>(
  tabs: T[],
  matchingTabs: Set<SettingsTab> | null,
): T[] {
  if (!matchingTabs) return tabs;
  return tabs.filter((item) => matchingTabs.has(item.id));
}

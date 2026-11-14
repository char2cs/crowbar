import { describe, expect, it } from 'vitest'
import { filterSettingsTabsBySearch } from '@/features/settings/lib/settings-tab-visibility'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'

const tabs = [{ id: 'appearance' }, { id: 'editor' }, { id: 'git' }] satisfies Array<{
  id: SettingsTab
}>

function visibleIds(matchingTabs: Set<SettingsTab> | null) {
  return filterSettingsTabsBySearch(tabs, matchingTabs).map((tab) => tab.id)
}

describe('filterSettingsTabsBySearch', () => {
  it('keeps every tab when no search is active', () => {
    expect(visibleIds(null)).toEqual(['appearance', 'editor', 'git'])
  })

  it('keeps only the tabs the search matched', () => {
    expect(visibleIds(new Set<SettingsTab>(['git']))).toEqual(['git'])
  })

  it('preserves the declared tab order rather than the match order', () => {
    expect(visibleIds(new Set<SettingsTab>(['git', 'appearance']))).toEqual(['appearance', 'git'])
  })

  it('returns nothing when the search matched no tab', () => {
    expect(visibleIds(new Set<SettingsTab>())).toEqual([])
  })
})

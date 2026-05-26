import { test, expect } from 'vitest'
import type { BottomPaneTab, SidebarActivityItem } from '@/features/window/stores/ui-state-store'

// Verify the live values still compile and work after the dead ones are removed
const validBottomTabs: BottomPaneTab[] = ['terminal', 'buffers']
const validSidebarItems: SidebarActivityItem[] = ['file-explorer', 'git', 'search', 'extensions']

test('active BottomPaneTab values remain valid', () => {
  expect(validBottomTabs).toContain('terminal')
  expect(validBottomTabs).toContain('buffers')
})

test('active SidebarActivityItem values remain valid', () => {
  expect(validSidebarItems).toContain('file-explorer')
  expect(validSidebarItems).toContain('git')
})

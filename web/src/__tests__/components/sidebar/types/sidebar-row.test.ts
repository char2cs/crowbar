import { describe, expect, it } from 'vitest'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

describe('SidebarRow', () => {
  it('the four kinds are the whole taxonomy', () => {
    const kinds: SidebarRow['kind'][] = ['chat', 'branch', 'folder', 'workflow']
    expect(kinds).toHaveLength(4)
  })
})

import { describe, expect, it } from 'vitest'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// REGRESSION (K3): the tree drew a tied level in arrival order while the drop
// planner counted it in another, and the daemon in a third — so the first
// drag out of an all-zero level landed slots off. One comparator now serves
// the tree (`SidebarTree`) and the planner (`renderedSiblings`), and it is
// the daemon's own (tree/node.go compareNodes): order, kind, createdAt, id.
function row(over: Partial<SidebarRow> & { id: string; kind: SidebarRow['kind'] }): SidebarRow {
  return {
    parentId: null,
    order: 0,
    label: over.id,
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
    ...over,
  }
}

describe('compareSidebarRows', () => {
  it('breaks an order tie by kind: folders, branches, chats, then repo headers', () => {
    const rows = [
      row({ id: 'chat', kind: 'chat', createdAt: '2026-01-01T00:00:01Z' }),
      row({
        id: 'repo',
        kind: 'branch',
        createdAt: '2026-01-01T00:00:02Z',
        repoIcon: { repoId: 'r', projectId: 'p', name: 'r', avatarLabel: 'R', avatarColor: '' },
      }),
      row({ id: 'branch', kind: 'branch', createdAt: '2026-01-01T00:00:03Z' }),
      row({ id: 'folder', kind: 'folder', createdAt: '2026-01-01T00:00:04Z' }),
    ]
    expect([...rows].sort(compareSidebarRows).map((r) => r.id)).toEqual([
      'folder',
      'branch',
      'chat',
      'repo',
    ])
  })

  it('breaks a same-kind tie by creation time, then id — never by arrival', () => {
    const rows = [
      row({ id: 'c-z', kind: 'chat', createdAt: '2026-01-01T00:00:03Z' }),
      row({ id: 'c-a', kind: 'chat', createdAt: '2026-01-01T00:00:03Z' }),
      row({ id: 'c-m', kind: 'chat', createdAt: '2026-01-01T00:00:01Z' }),
    ]
    expect([...rows].sort(compareSidebarRows).map((r) => r.id)).toEqual(['c-m', 'c-a', 'c-z'])
  })

  it('a dense order always wins over kind and creation time', () => {
    const rows = [
      row({ id: 'folder', kind: 'folder', order: 2, createdAt: '2026-01-01T00:00:01Z' }),
      row({ id: 'chat', kind: 'chat', order: 0, createdAt: '2026-01-01T00:00:09Z' }),
      row({ id: 'branch', kind: 'branch', order: 1 }),
    ]
    expect([...rows].sort(compareSidebarRows).map((r) => r.id)).toEqual([
      'chat',
      'branch',
      'folder',
    ])
  })
})

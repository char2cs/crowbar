import { describe, expect, it } from 'vitest'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

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

  it('breaks a tie between two repo headers on the REPO id, not the owning-chat row id', () => {
    const header = (id: string, repoId: string) =>
      row({
        id,
        kind: 'branch',
        repoIcon: { repoId, projectId: 'p', name: repoId, avatarLabel: 'R', avatarColor: '' },
      })
    const rows = [header('zz-owner', '0a-alpha'), header('aa-owner', 'f9-beta')]
    expect([...rows].sort(compareSidebarRows).map((r) => r.repoIcon?.repoId)).toEqual([
      '0a-alpha',
      'f9-beta',
    ])
    expect(
      [...rows]
        .reverse()
        .sort(compareSidebarRows)
        .map((r) => r.repoIcon?.repoId),
    ).toEqual(['0a-alpha', 'f9-beta'])
  })

  it('breaks a creation-time tie on the instant, not the ISO string', () => {
    const shorterFraction = row({ id: 'z', kind: 'chat', createdAt: '2026-09-17T20:36:46.5Z' })
    const longerFraction = row({ id: 'a', kind: 'chat', createdAt: '2026-09-17T20:36:46.53Z' })
    expect([shorterFraction, longerFraction].sort(compareSidebarRows).map((r) => r.id)).toEqual([
      'z',
      'a',
    ])
    expect([longerFraction, shorterFraction].sort(compareSidebarRows).map((r) => r.id)).toEqual([
      'z',
      'a',
    ])

    const earlierInstant = row({ id: 'z', kind: 'chat', createdAt: '2026-11-01T01:30:00-07:00' })
    const laterInstant = row({ id: 'a', kind: 'chat', createdAt: '2026-11-01T01:15:00-08:00' })
    expect(compareSidebarRows(earlierInstant, laterInstant)).toBeLessThan(0)
    expect(compareSidebarRows(laterInstant, earlierInstant)).toBeGreaterThan(0)
  })

  it('a row with no createdAt sorts as the daemon’s zero time — before every real instant', () => {
    const unstamped = row({ id: 'z', kind: 'chat' })
    const stamped = row({ id: 'a', kind: 'chat', createdAt: '2026-01-01T00:00:00Z' })
    expect([stamped, unstamped].sort(compareSidebarRows).map((r) => r.id)).toEqual(['z', 'a'])
    const zeroTime = row({ id: 'm', kind: 'chat', createdAt: '0001-01-01T00:00:00Z' })
    expect([zeroTime, unstamped].sort(compareSidebarRows).map((r) => r.id)).toEqual(['m', 'z'])
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

import { describe, expect, it } from 'vitest'
import { rowVerbs } from '@/components/sidebar/lib/row-menu'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const base: SidebarRow = {
  id: 'row-1',
  kind: 'chat',
  parentId: null,
  order: 0,
  label: 'Row',
  ownsWorktree: false,
  workspaceId: 'ws-1',
  working: false,
  hasView: false,
}
const branch: SidebarRow = {
  ...base,
  kind: 'branch',
  ownsWorktree: true,
  branchName: 'feature/x',
  locked: false,
  isPlaceholder: false,
}
const ROWS: Record<string, { row: SidebarRow; locked?: boolean }> = {
  chat: { row: base },
  'thread in a repo': { row: { ...base, canBranchOut: true } },
  'folder in a repo': { row: { ...base, kind: 'folder', ownsWorktree: true } },
  'folder at home': { row: { ...base, kind: 'folder', ownsWorktree: false } },
  branch: { row: branch },
  'locked branch': { row: { ...branch, locked: true, status: 'locked' }, locked: true },
  'placeholder branch': { row: { ...branch, isPlaceholder: true } },
  'held branch': { row: { ...branch, isPlaceholder: true, heldByPath: '/elsewhere' } },
  'project home': {
    row: {
      ...branch,
      repoIcon: {
        repoId: 'repo-1',
        projectId: 'p1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: '#000',
      },
    },
  },
}

describe('rowVerbs', () => {
  for (const [kind, { row, locked = false }] of Object.entries(ROWS)) {
    it(`marks a ${kind} row's destructive verbs and puts them last`, () => {
      const verbs = rowVerbs(row, { locked, canStartOnTerminal: true, movePending: false })
      const destructive = verbs.map((v) => v.destructive)
      expect(destructive).toEqual([...destructive].sort((a, b) => Number(a) - Number(b)))
      for (const verb of verbs) {
        expect(verb.destructive).toBe(verb.id === 'remove' || verb.id === 'delete-repo')
      }
    })
  }

  it('lets the user remove a chat, a folder and an unlocked branch', () => {
    for (const row of [ROWS.chat.row, ROWS['folder in a repo'].row, ROWS.branch.row]) {
      const ids = rowVerbs(row, {
        locked: false,
        canStartOnTerminal: true,
        movePending: false,
      }).map((v) => v.id)
      expect(ids.at(-1)).toBe('remove')
    }
  })

  it('offers Make a branch on a repo thread, but not at home nor while it already moves', () => {
    const ids = (row: SidebarRow, movePending: boolean) =>
      rowVerbs(row, { locked: false, canStartOnTerminal: true, movePending }).map((v) => v.id)
    expect(ids({ ...base, canBranchOut: true }, false)).toContain('branch-out')
    expect(ids({ ...base, canBranchOut: false }, false)).not.toContain('branch-out')
    expect(ids({ ...base, canBranchOut: true }, true)).not.toContain('branch-out')
  })
})

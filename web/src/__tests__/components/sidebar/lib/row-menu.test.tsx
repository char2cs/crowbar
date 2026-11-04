import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SidebarRowActions } from '@/components/sidebar/sidebar-row-actions'
import { rowMenuActions, rowVerbs, type RowMenuActionId } from '@/components/sidebar/lib/row-menu'
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

const CONTROL_VERB: Record<string, RowMenuActionId> = {
  detach: 'detach',
  'retry-provision': 'retry',
  thread: 'new-thread',
  fork: 'fork',
  remove: 'remove',
}

// Everything the desktop offers a row: its right-click menu plus its hover buttons.
function desktopVerbs(row: SidebarRow, locked: boolean): RowMenuActionId[] {
  const { container, unmount } = render(
    <SidebarRowActions
      row={row}
      isProjectHome={row.repoIcon !== undefined}
      expanded={false}
      subActionClass=""
      onCreate={vi.fn()}
      onTrash={vi.fn()}
      onToggleFold={vi.fn()}
    />,
  )
  const buttons = [...container.querySelectorAll('[data-control]')]
    .map((el) => CONTROL_VERB[el.getAttribute('data-control') ?? ''])
    .filter(Boolean)
  unmount()
  const menu = rowMenuActions(row, { locked, canStartOnTerminal: true })
  const all = [...menu.map((a) => a.id), ...buttons]
  const destructive = new Set<RowMenuActionId>(['remove', 'delete-repo'])
  return [...all.filter((id) => !destructive.has(id)), ...all.filter((id) => destructive.has(id))]
}

describe('rowVerbs', () => {
  for (const [kind, { row, locked = false }] of Object.entries(ROWS)) {
    it(`gives a ${kind} row every verb the desktop does, destructive last`, () => {
      const verbs = rowVerbs(row, { locked, canStartOnTerminal: true })
      expect(verbs.map((v) => v.id)).toEqual(desktopVerbs(row, locked))
      for (const verb of verbs) {
        expect(verb.destructive).toBe(verb.id === 'remove' || verb.id === 'delete-repo')
      }
    })
  }

  it('lets the user remove a chat, a folder and an unlocked branch', () => {
    for (const row of [ROWS.chat.row, ROWS['folder in a repo'].row, ROWS.branch.row]) {
      const ids = rowVerbs(row, { locked: false, canStartOnTerminal: true }).map((v) => v.id)
      expect(ids.at(-1)).toBe('remove')
    }
  })
})

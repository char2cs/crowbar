import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

const touch = vi.hoisted(() => ({ value: true }))
vi.mock('@/hooks/use-is-touch-device', () => ({ useIsTouchDevice: () => touch.value }))
vi.mock('@/components/sidebar/sidebar-tree', () => ({
  SidebarTreeRow: ({ row }: { row: { id: string; label: string } }) => (
    <div data-testid="tree-row" data-row-id={row.id}>
      {row.label}
    </div>
  ),
}))

import { ActiveSessionsSection } from '@/components/sidebar/active-sessions-section'
import { useActiveSessionsStore } from '@/lib/store/active-sessions'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const row = (id: string, kind: SidebarRow['kind'] = 'chat'): SidebarRow => ({
  id,
  kind,
  parentId: null,
  order: 0,
  label: `Row ${id}`,
  ownsWorktree: false,
  workspaceId: 'ws-1',
  working: false,
  hasView: false,
})

const run = (id: string) =>
  useActiveSessionsStore.getState().applyFrame('repo:r1', {
    chatId: id,
    kind: 'started',
    chat: { id, workspaceId: 'ws-1', title: id, liveRunnerId: 'run' },
  })

const renderSection = (rows: SidebarRow[]) =>
  render(
    <ActiveSessionsSection rows={rows} onOpen={vi.fn()} onTrash={vi.fn()} onCreate={vi.fn()} />,
  )

beforeEach(() => {
  touch.value = true
  useActiveSessionsStore.setState({ sessions: {}, attached: {} })
})

describe('ActiveSessionsSection', () => {
  it('lists the tree’s own rows for the chats whose agents are running, with their viewers', () => {
    run('c1')
    run('c2')
    useActiveSessionsStore
      .getState()
      .applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 2 })

    renderSection([row('c1'), row('c2'), row('c3')])

    expect(screen.getAllByTestId('tree-row').map((el) => el.dataset.rowId)).toEqual(['c1', 'c2'])
    expect(screen.getByText('2 viewers')).toBeTruthy()
    expect(screen.getByText('No viewers')).toBeTruthy()
  })

  it('draws nothing on a device without touch', () => {
    run('c1')
    touch.value = false

    renderSection([row('c1')])

    expect(screen.queryByTestId('active-sessions-section')).toBeNull()
  })

  it('draws nothing when no listed row has a running agent', () => {
    run('elsewhere')

    renderSection([row('c1')])

    expect(screen.queryByTestId('active-sessions-section')).toBeNull()
  })

  it('never lists a folder', () => {
    run('f1')

    renderSection([row('f1', 'folder')])

    expect(screen.queryByTestId('active-sessions-section')).toBeNull()
  })
})

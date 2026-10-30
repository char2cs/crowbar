import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const ROW: SidebarRow = {
  id: 'thread-1',
  kind: 'chat',
  parentId: null,
  order: 0,
  label: 'A thread',
  ownsWorktree: false,
  workspaceId: null,
  working: false,
  hasView: true,
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  windowPaneStore.getState().paneActions.openChat('thread-1')
})

function rightClick(inStrip: boolean) {
  const tree = document.createElement('div')
  document.body.appendChild(tree)
  const host = document.createElement('div')
  if (inStrip) host.setAttribute('data-view-tab-strip', '')
  tree.appendChild(host)
  render(<SidebarRowContextMenu treeRef={{ current: tree }} rows={[ROW]} onImport={vi.fn()} />)
  const target = document.createElement('div')
  target.setAttribute('role', 'treeitem')
  target.setAttribute('data-sidebar-row-id', ROW.id)
  host.appendChild(target)
  fireEvent.contextMenu(target)
}
const viewOf = () => Object.values(windowPaneStore.getState().views)[0]

describe('a view tab in the strip', () => {
  it('offers Pin tab, which pins its view', () => {
    rightClick(true)
    fireEvent.click(screen.getByText('Pin tab'))
    expect(viewOf().pinned).toBe(true)
  })

  it('offers Unpin tab once pinned', () => {
    windowPaneStore.getState().paneActions.setViewPinned(viewOf().id, true)
    rightClick(true)
    expect(screen.queryByText('Pin tab')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Unpin tab'))
    expect(viewOf().pinned).toBeUndefined()
  })

  it('offers Close tab, which ends the view', () => {
    rightClick(true)
    fireEvent.click(screen.getByText('Close tab'))
    expect(Object.keys(windowPaneStore.getState().views)).toHaveLength(0)
  })

  it('is the only row that does: the same chat in the sidebar tree has neither', () => {
    rightClick(false)
    expect(screen.queryByText('Pin tab')).not.toBeInTheDocument()
    expect(screen.queryByText('Close tab')).not.toBeInTheDocument()
  })
})

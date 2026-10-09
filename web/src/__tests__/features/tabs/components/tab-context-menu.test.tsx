import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import TabContextMenu from '@/features/tabs/components/tab-context-menu'
import type { EditorContent } from '@/features/panes/types/pane-content'

const buffer: EditorContent = {
  id: 'buf-1',
  type: 'editor',
  path: '/project/a.ts',
  name: 'a.ts',
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  tokens: [],
  workspaceId: 'w1',
}

function renderMenu(onRevealInFinder?: (path: string) => void) {
  render(
    <TabContextMenu
      isOpen
      position={{ x: 0, y: 0 }}
      buffer={buffer}
      onClose={vi.fn()}
      onPin={vi.fn()}
      onCloseTab={vi.fn()}
      onCloseOthers={vi.fn()}
      onCloseAll={vi.fn()}
      onCloseToRight={vi.fn()}
      onRevealInFinder={onRevealInFinder}
    />,
  )
}

describe('TabContextMenu', () => {
  it('offers Reveal in Finder when the host hands it a way to', () => {
    renderMenu(vi.fn())

    expect(screen.getByText('Reveal in Finder')).toBeInTheDocument()
  })

  // No handler means the tab's files are not on this machine: an item that does nothing
  // would be a row that lies.
  it('omits it, rather than showing a dead row, when there is no way to', () => {
    renderMenu(undefined)

    expect(screen.getByText('Copy Path')).toBeInTheDocument()
    expect(screen.queryByText('Reveal in Finder')).toBeNull()
  })
})

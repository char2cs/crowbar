import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TabBarItem from '@/features/tabs/components/tab-bar-item'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => <span data-testid="file-icon" />,
}))

const editorBuffer: EditorContent = {
  id: 'buf-1',
  type: 'editor',
  authority: 'workspace',
  path: '/project/bar.ts',
  name: 'bar.ts',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  workspaceId: 'w1',
}

const shared = {
  displayName: 'bar.ts',
  index: 0,
  isDraggedTab: false,
  onDoubleClick: () => {},
  onContextMenu: () => {},
  onKeyDown: () => {},
  handleTabClose: () => {},
  handleTabPin: () => {},
}

describe('TabBarItem', () => {
  // Spec §7.1: there is no "Editor"/New Tab placeholder tab any more — the
  // sole-tab-in-a-pane invariant (isUncloseable) now applies to any real
  // editor-tab content, exercised here with a plain editor buffer.
  it('renders an uncloseable tab with its label and no close button', () => {
    const buffer: EditorContent = { ...editorBuffer, isUncloseable: true }
    render(<TabBarItem buffer={buffer} isActive {...shared} />)
    expect(screen.getByText('bar.ts')).toBeInTheDocument()
    expect(screen.queryByLabelText(/close/i)).not.toBeInTheDocument()
  })

  it('an inactive dirty tab shows the unsaved dot', () => {
    const buffer: EditorContent = { ...editorBuffer, isDirty: true }
    render(<TabBarItem buffer={buffer} isActive={false} {...shared} />)
    expect(screen.getByLabelText('Unsaved changes')).toBeInTheDocument()
  })
})

// A locked branch's file says so on its tab before the user types, not only when a save fails.
describe('TabBarItem on a locked workspace', () => {
  it('marks the tab read-only', () => {
    seedRepos([
      {
        id: 'r1',
        projectId: 'p1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: 'r',
        workspaces: [],
        defaultWorkspaceId: 'w1',
        defaultWorkspaceStatus: 'locked',
      },
    ])
    render(<TabBarItem buffer={editorBuffer} isActive {...shared} />)

    expect(screen.getByRole('tab', { name: 'bar.ts (read-only)' })).toBeInTheDocument()
    resetEntities()
  })

  it('leaves an unlocked workspace’s tab alone', () => {
    render(<TabBarItem buffer={editorBuffer} isActive {...shared} />)

    expect(screen.getByRole('tab', { name: 'bar.ts' })).toBeInTheDocument()
  })
})

describe('TabBarItem title', () => {
  it('exposes the full title of a long tab to assistive tech and the hover tooltip', async () => {
    const title = 'Crowbar - Bring your own agent. Keep your own context. Always.'
    const terminalBuffer = {
      id: 't-1',
      type: 'terminal',
      sessionId: 'pty-1',
      name: title,
      path: 'terminal://pty-1',
      isPinned: false,
      isPreview: false,
      workspaceId: 'w1',
    } as const
    render(<TabBarItem buffer={terminalBuffer} isActive {...shared} displayName={title} />)

    const tab = screen.getByRole('tab', { name: title })
    expect(tab.textContent).toBe(title)
    expect(screen.getAllByRole('button')).toHaveLength(1)

    fireEvent.pointerEnter(tab)
    fireEvent.mouseEnter(tab)
    fireEvent.focus(tab)
    expect(
      await screen.findByText(title, { selector: '[data-slot="tooltip-content"] *' }),
    ).toBeTruthy()
  })
})

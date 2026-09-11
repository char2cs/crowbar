import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TabBarItem from '@/features/tabs/components/tab-bar-item'
import type { EditorContent } from '@/features/panes/types/pane-content'

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => <span data-testid="file-icon" />,
}))

const editorBuffer: EditorContent = {
  id: 'buf-1',
  type: 'editor',
  path: '/project/bar.ts',
  name: 'bar.ts',
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  tokens: [],
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

describe('TabBarItem ghost restyle', () => {
  it('active tab is flat, not a filled rounded pill', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).not.toHaveClass('rounded-full')
    expect(tab).not.toHaveClass('bg-background')
    expect(tab).not.toHaveClass('border-background')
  })

  it('inactive tab is flat with muted text, no fill', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={false} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).not.toHaveClass('rounded-full')
    expect(tab).not.toHaveClass('bg-background')
    expect(tab).toHaveClass('text-muted-foreground')
  })

  // The IDE sector's tab strip reads as the same ghost-toolbar-button family
  // as the split-toggle/close-view/add-tab buttons in the same row — no
  // underline bar, a persistent sidebar-element-hover fill on the active tab.
  it('active tab carries a persistent sidebar-element-hover fill, not an underline bar', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).toHaveClass('bg-sidebar-element-hover')
    expect(tab).toHaveClass('text-foreground')
    expect(screen.queryByTestId('tab-underline')).not.toBeInTheDocument()
  })

  it('inactive tab has no fill and no underline bar', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={false} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).not.toHaveClass('bg-sidebar-element-hover')
    expect(screen.queryByTestId('tab-underline')).not.toBeInTheDocument()
  })

  it('active tab does not have bg-foreground/85 (old pill style removed)', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).not.toHaveClass('bg-foreground/85')
  })

  it('close button is a small rounded-md control', () => {
    const { container } = render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    // The Tab <button> (role=tab) is buttons[0]; the close Button sibling is buttons[1]
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('!rounded-md')
  })

  it('close button has hover:bg-accent class regardless of active state', () => {
    const { container } = render(<TabBarItem buffer={editorBuffer} isActive={false} {...shared} />)
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('hover:bg-accent')
  })

  it('close button has opacity-60 when tab is active', () => {
    const { container } = render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('opacity-60')
    expect(closeBtn).not.toHaveClass('opacity-100')
  })

  // Spec §7.1: there is no "Editor"/New Tab placeholder tab any more — the
  // sole-tab-in-a-pane invariant (isUncloseable) now applies to any real
  // editor-tab content, exercised here with a plain editor buffer.
  it('renders an uncloseable tab with its label and no close button', () => {
    const buffer: EditorContent = { ...editorBuffer, isUncloseable: true }
    render(<TabBarItem buffer={buffer} isActive {...shared} />)
    expect(screen.getByText('bar.ts')).toBeInTheDocument()
    expect(screen.queryByLabelText(/close/i)).not.toBeInTheDocument()
  })
})

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
  isActive: false,
  tokens: [],
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

describe('TabBarItem pill restyle', () => {
  it('active tab has rounded-lg and outline variant classes', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).toHaveClass('rounded-lg')
    expect(tab).toHaveClass('border-input')
  })

  it('inactive tab has ghost variant classes', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={false} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).toHaveClass('rounded-lg')
    expect(tab).toHaveClass('border-transparent')
  })

  it('active tab does not have bg-foreground/85 (old pill style removed)', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).not.toHaveClass('bg-foreground/85')
  })

  it('close button has rounded-full class', () => {
    const { container } = render(
      <TabBarItem buffer={editorBuffer} isActive={true} {...shared} />
    )
    // The Tab <button> (role=tab) is buttons[0]; the close Button sibling is buttons[1]
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('rounded-full')
  })

  it('close button has hover:bg-accent class regardless of active state', () => {
    const { container } = render(
      <TabBarItem buffer={editorBuffer} isActive={false} {...shared} />
    )
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
})

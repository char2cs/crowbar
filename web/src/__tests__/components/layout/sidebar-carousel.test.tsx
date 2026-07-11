import React from 'react'
import { act, render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="panel-workspaces" />,
}))
vi.mock('@/features/agent/components/agent-chats-panel', () => ({
  AgentChatsPanel: () => <div data-testid="panel-chats" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="panel-files" />,
}))
vi.mock('@/features/git/components/git-panel', () => ({
  GitPanel: () => <div data-testid="panel-git" />,
}))
vi.mock('@/components/error-boundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: (state: unknown) => unknown) =>
      sel({ files: [], handleFileOpen: null, handleFileSelect: null }),
    { use: { handleFileOpen: () => null, handleFileSelect: () => null } },
  ),
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))
vi.mock('@/lib/store/sidebar', async () => {
  const actual = await vi.importActual('@/lib/store/sidebar')
  return actual
})

describe('SidebarCarousel', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
  })

  it('mounts all 4 panels', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    expect(screen.getByTestId('panel-workspaces')).toBeInTheDocument()
    expect(screen.getByTestId('panel-chats')).toBeInTheDocument()
    expect(screen.getByTestId('panel-files')).toBeInTheDocument()
    expect(screen.getByTestId('panel-git')).toBeInTheDocument()
  })

  it('renders the panels in Workspaces, Chats, Files, Git order (index math must not be off-by-one)', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen.getByTestId('panel-workspaces').closest('[data-sidebar-carousel]')
    const testIds = Array.from(container?.querySelectorAll('[data-testid^="panel-"]') ?? []).map(
      (el) => el.getAttribute('data-testid'),
    )
    expect(testIds).toEqual(['panel-workspaces', 'panel-chats', 'panel-files', 'panel-git'])
  })

  it('scrolls to the Chats panel index (1) when activeTab is chats', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen
      .getByTestId('panel-workspaces')
      .closest('[data-sidebar-carousel]') as HTMLElement
    Object.defineProperty(container, 'clientWidth', { value: 400, configurable: true })
    const scrollToSpy = HTMLElement.prototype.scrollTo as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    act(() => {
      useSidebarStore.setState({ activeTab: 'chats' })
    })

    expect(scrollToSpy).toHaveBeenCalledWith(
      expect.objectContaining({ left: 400, behavior: 'smooth' }),
    )
  })
})

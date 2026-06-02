import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="panel-workspaces" />,
}))
vi.mock('@/features/chats/components/chat-list', () => ({
  ChatList: () => <div data-testid="panel-chats" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="panel-files" />,
}))
vi.mock('@/features/git/components/git-panel', () => ({
  GitPanel: () => <div data-testid="panel-git" />,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: any) => sel({ files: [], handleFileOpen: null, handleFileSelect: null }),
    { use: { handleFileOpen: () => null, handleFileSelect: () => null } },
  ),
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))

describe('SidebarCarousel', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
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
})

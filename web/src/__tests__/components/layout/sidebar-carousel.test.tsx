import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useRouterState: (opts: { select: (state: unknown) => unknown }) => {
      const select = opts.select
      return select({ location: { pathname: '/workspaces/test-ws' } })
    },
  }
})

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="panel-workspaces" />,
}))
vi.mock('@/components/layout/chat-tree', () => ({
  ChatTree: () => <div data-testid="panel-chats" />,
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
})

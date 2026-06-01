import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { SidebarTabs } from '@/components/layout/SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="workspaces-panel" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="files-panel" />,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/components/layout/SidebarSkeleton', () => ({
  SidebarSkeleton: () => null,
}))
vi.mock('@/lib/mock/files', () => ({
  getMockFileTree: () => [],
}))
vi.mock('@/features/file-system/controllers/store', () => {
  const store = Object.assign(
    (selector: (s: { files: never[]; handleFileOpen: null; handleFileSelect: null }) => unknown) =>
      selector({ files: [], handleFileOpen: null, handleFileSelect: null }),
    { use: { handleFileOpen: () => null, handleFileSelect: () => null } }
  )
  return { useFileSystemStore: store }
})
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))

const defaultProps = {
  activeWorkspaceRepoPath: '/repos/default',
}

describe('SidebarTabs', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('does not render text tab triggers', () => {
    useSidebarStore.setState({ activeTab: 'workspaces' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.queryByRole('tab', { name: 'Workspaces' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Files' })).not.toBeInTheDocument()
  })

  it('shows workspaces panel when activeTab is workspaces', () => {
    useSidebarStore.setState({ activeTab: 'workspaces' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.getByTestId('workspaces-panel')).toBeVisible()
    expect(screen.getByTestId('files-panel')).not.toBeVisible()
  })

  it('shows files panel when activeTab is files', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.getByTestId('files-panel')).toBeVisible()
    expect(screen.getByTestId('workspaces-panel')).not.toBeVisible()
  })
})

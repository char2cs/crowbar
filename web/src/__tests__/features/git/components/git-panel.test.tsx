import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'

vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="file-explorer" />,
}))
vi.mock('@/features/git/components/git-history-list', () => ({
  GitHistoryList: () => <div data-testid="git-history" />,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: any) => sel({ files: [], handleFileOpen: null }),
    { use: { handleFileOpen: () => null } },
  ),
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))

describe('GitPanel', () => {
  it('renders Changes and History tabs', () => {
    render(<GitPanel />)
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /history/i })).toBeInTheDocument()
  })

  it('shows the file explorer in the Changes tab by default', () => {
    render(<GitPanel />)
    expect(screen.getByTestId('file-explorer')).toBeInTheDocument()
  })
})

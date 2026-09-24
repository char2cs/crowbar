import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FileExplorerTree } from '@/features/file-explorer/file-explorer/components/file-explorer-tree'
import type { FileEntry } from '@/features/file-system/types/app'

vi.mock('@/features/file-system/controllers/platform', () => ({
  readDirectory: vi.fn(async () => []),
  readFile: vi.fn(async () => ''),
}))

const files: FileEntry[] = [
  { name: 'a.ts', path: 'a.ts' },
  { name: 'b.ts', path: 'b.ts' },
  { name: 'c.ts', path: 'c.ts' },
]

function renderTree(onFileOpen = vi.fn()) {
  render(
    <FileExplorerTree
      workspaceId="ws1"
      files={files}
      rootFolderPath="/repos/r1"
      onFileSelect={vi.fn()}
      onFileOpen={onFileOpen}
      onCreateNewFileInDirectory={vi.fn()}
    />,
  )
  return screen.getByRole('tree')
}

describe('FileExplorerTree keyboard', () => {
  it('moves the cursor with the arrow keys and opens the file on Enter', () => {
    const onFileOpen = vi.fn()
    const tree = renderTree(onFileOpen)
    fireEvent.focus(tree)
    expect(tree.getAttribute('aria-activedescendant')).toBe('file-tree-row-a_ts')

    fireEvent.keyDown(tree, { key: 'ArrowDown' })
    fireEvent.keyDown(tree, { key: 'ArrowDown' })
    expect(tree.getAttribute('aria-activedescendant')).toBe('file-tree-row-c_ts')

    fireEvent.keyDown(tree, { key: 'Home' })
    fireEvent.keyDown(tree, { key: 'ArrowDown' })
    fireEvent.keyDown(tree, { key: 'Enter' })
    expect(onFileOpen).toHaveBeenCalledWith('b.ts', false)
  })

  it('opens the filter box on "/" and closes it on Escape', () => {
    const tree = renderTree()
    expect(screen.queryByLabelText('Filter files in tree')).toBeNull()

    fireEvent.keyDown(tree, { key: '/' })
    const input = screen.getByLabelText('Filter files in tree')
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.queryByLabelText('Filter files in tree')).toBeNull()
  })

  it('opens the filter box on the file-tree-open-search event', () => {
    renderTree()
    fireEvent(window, new Event('file-tree-open-search'))
    expect(screen.getByLabelText('Filter files in tree')).toBeTruthy()
  })
})

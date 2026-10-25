import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FileExplorerTree } from '@/features/file-explorer/file-explorer/components/file-explorer-tree'
import type { FileEntry } from '@/features/file-system/types/app'
import { SIDEBAR_FILE_ROW_HEIGHT } from '@/components/sidebar/lib/sidebar-file-row-height'

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
      isLoading={false}
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

  it('puts the cursor on the first match while filtering, and keeps it there on Escape', async () => {
    const tree = renderTree()
    fireEvent.focus(tree)
    fireEvent.keyDown(tree, { key: '/' })
    const input = screen.getByLabelText('Filter files in tree')
    fireEvent.change(input, { target: { value: 'b' } })
    await waitFor(() =>
      expect(tree.getAttribute('aria-activedescendant')).toBe('file-tree-row-b_ts'),
    )

    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.queryByLabelText('Filter files in tree')).toBeNull()
    expect(tree.getAttribute('aria-activedescendant')).toBe('file-tree-row-b_ts')
  })
})

describe('FileExplorerTree virtualisation', () => {
  // jsdom has no layout: give elements a sidebar-sized rect so the virtualiser has a window.
  const originalRect = HTMLElement.prototype.getBoundingClientRect
  beforeEach(() => {
    const rect = { top: 0, left: 0, right: 280, bottom: 400, width: 280, height: 400, x: 0, y: 0 }
    HTMLElement.prototype.getBoundingClientRect = () => ({ ...rect, toJSON: () => rect }) as DOMRect
  })
  afterEach(() => {
    HTMLElement.prototype.getBoundingClientRect = originalRect
  })

  it('lays rows out on the 26px pitch the rows themselves render at', () => {
    const many: FileEntry[] = Array.from({ length: 300 }, (_, i) => ({
      name: `f${String(i).padStart(3, '0')}.ts`,
      path: `f${String(i).padStart(3, '0')}.ts`,
    }))
    const { container } = render(
      <FileExplorerTree
        workspaceId="ws-virtual"
        files={many}
        isLoading={false}
        rootFolderPath="/repos/r1"
        onFileSelect={vi.fn()}
        onFileOpen={vi.fn()}
        onCreateNewFileInDirectory={vi.fn()}
      />,
    )
    const rows = screen.getAllByRole('treeitem')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(many.length)
    for (const row of rows) expect(row.style.height).toBe(`${SIDEBAR_FILE_ROW_HEIGHT}px`)

    const body = container.querySelector('#file-tree-results') as HTMLElement
    const spacers = [body.firstElementChild, body.lastElementChild] as HTMLElement[]
    const spacerHeight = spacers.reduce((sum, el) => sum + Number.parseFloat(el.style.height), 0)
    expect(spacerHeight + rows.length * SIDEBAR_FILE_ROW_HEIGHT).toBe(
      many.length * SIDEBAR_FILE_ROW_HEIGHT,
    )
  })
})

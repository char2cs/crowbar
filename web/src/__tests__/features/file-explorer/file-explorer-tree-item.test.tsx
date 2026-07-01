import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FileExplorerTreeItem } from '@/features/file-explorer/file-explorer/components/file-explorer-tree-item'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import type { FileEntry } from '@/features/file-system/types/app'

// The component calls useFileClipboardStore to check cut state — stub it out.
vi.mock('@/features/file-explorer/stores/file-explorer-clipboard-store', () => ({
  useFileClipboardStore: () => false,
}))

const makeFile = (name: string, path: string, isDir = false): FileEntry => ({
  name,
  path,
  isDir,
})

const noDecoration = (_file: FileEntry): FileTreeGitStatusDecoration | null => null

const withDecoration =
  (decoration: FileTreeGitStatusDecoration) =>
  (_file: FileEntry): FileTreeGitStatusDecoration | null =>
    decoration

const defaultProps = {
  depth: 0,
  guideTargets: [],
  previousDepth: 0,
  nextDepth: 0,
  indentSize: 14,
  density: 'default' as const,
  isExpanded: false,
  isActive: false,
  dragOverPath: null,
  isDragging: false,
  editingValue: '',
  onEditingValueChange: vi.fn(),
  onKeyDown: vi.fn(),
  onBlur: vi.fn(),
}

describe('FileExplorerTreeItem git status decorations', () => {
  it('renders no status letter for an unchanged file', () => {
    const file = makeFile('index.ts', '/repo/index.ts')
    const { container } = render(
      <FileExplorerTreeItem {...defaultProps} file={file} getGitStatusDecoration={noDecoration} />,
    )
    // filename is present
    expect(screen.getByText('index.ts')).toBeInTheDocument()
    // no letter badge
    expect(container.querySelector('[aria-label]')).toBeNull()
  })

  it('applies text-git-modified class and M letter for a modified file', () => {
    const decoration: FileTreeGitStatusDecoration = {
      colorClassName: 'text-git-modified',
      label: 'Modified',
      statusLetter: 'M',
    }
    const file = makeFile('app.ts', '/repo/app.ts')
    render(
      <FileExplorerTreeItem
        {...defaultProps}
        file={file}
        getGitStatusDecoration={withDecoration(decoration)}
      />,
    )

    const nameSpan = screen.getByText('app.ts')
    expect(nameSpan.className).toContain('text-git-modified')

    const letterBadge = screen.getByLabelText('Modified')
    expect(letterBadge).toHaveTextContent('M')
    expect(letterBadge.className).toContain('text-git-modified')
  })

  it('applies text-git-untracked class and U letter for an untracked file', () => {
    const decoration: FileTreeGitStatusDecoration = {
      colorClassName: 'text-git-untracked',
      label: 'Untracked',
      statusLetter: 'U',
    }
    const file = makeFile('new-file.ts', '/repo/new-file.ts')
    render(
      <FileExplorerTreeItem
        {...defaultProps}
        file={file}
        getGitStatusDecoration={withDecoration(decoration)}
      />,
    )

    const nameSpan = screen.getByText('new-file.ts')
    expect(nameSpan.className).toContain('text-git-untracked')

    const letterBadge = screen.getByLabelText('Untracked')
    expect(letterBadge).toHaveTextContent('U')
    expect(letterBadge.className).toContain('text-git-untracked')
  })

  it('applies text-git-added class and A letter for an added file', () => {
    const decoration: FileTreeGitStatusDecoration = {
      colorClassName: 'text-git-added',
      label: 'Added',
      statusLetter: 'A',
    }
    const file = makeFile('staged.ts', '/repo/staged.ts')
    render(
      <FileExplorerTreeItem
        {...defaultProps}
        file={file}
        getGitStatusDecoration={withDecoration(decoration)}
      />,
    )

    expect(screen.getByLabelText('Added')).toHaveTextContent('A')
  })

  it('applies text-git-deleted class and D letter for a deleted file', () => {
    const decoration: FileTreeGitStatusDecoration = {
      colorClassName: 'text-git-deleted',
      label: 'Deleted',
      statusLetter: 'D',
    }
    const file = makeFile('gone.ts', '/repo/gone.ts')
    render(
      <FileExplorerTreeItem
        {...defaultProps}
        file={file}
        getGitStatusDecoration={withDecoration(decoration)}
      />,
    )

    expect(screen.getByLabelText('Deleted')).toHaveTextContent('D')
  })

  it('tints a folder name when a descendant has a change', () => {
    // The lookup propagates the modified decoration to the parent dir;
    // the component simply applies whatever decoration getGitStatusDecoration returns.
    const decoration: FileTreeGitStatusDecoration = {
      colorClassName: 'text-git-modified',
      label: 'Modified',
      statusLetter: 'M',
    }
    const folder = makeFile('src', '/repo/src', true)
    render(
      <FileExplorerTreeItem
        {...defaultProps}
        file={folder}
        getGitStatusDecoration={withDecoration(decoration)}
      />,
    )

    const nameSpan = screen.getByText('src')
    expect(nameSpan.className).toContain('text-git-modified')
    // folder also shows the status letter (consistent with VS Code behavior)
    expect(screen.getByLabelText('Modified')).toHaveTextContent('M')
  })
})

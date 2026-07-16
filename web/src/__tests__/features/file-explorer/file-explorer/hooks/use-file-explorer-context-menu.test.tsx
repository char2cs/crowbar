import { act, renderHook } from '@testing-library/react'
import { isValidElement, type ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useFileExplorerContextMenu } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { joinPath } from '@/utils/path-helpers'

const ROOT = '/root'

// Base options: every required callback stubbed. Individual tests override the
// handler + gating fields they exercise. rootFolderPath drives the absolute-path
// join that Reveal in Finder and Copy Path perform.
function baseOptions() {
  return {
    rootFolderPath: ROOT,
    isLocked: false,
    onFileSelect: vi.fn(),
    onCreateNewFileInDirectory: vi.fn(),
    onDeleteRequested: vi.fn(),
    onStartInlineEditing: vi.fn(),
    onOpenAllFilesInDirectory: vi.fn().mockResolvedValue(undefined),
    onUploadFile: vi.fn(),
    onDuplicatePath: vi.fn(),
    onRevealInFinder: vi.fn(),
    onAddFolderToWorkspace: vi.fn(),
    onRemoveFolderFromWorkspace: vi.fn(),
    onRenamePath: vi.fn(),
    isWorkspaceRootPath: (_: string) => false,
    canRemoveWorkspaceRootPath: (_: string) => false,
  }
}

// The hook returns a React element tree, not the raw item list; walk it to find
// the ContextMenu element and read its `items` prop. This avoids rendering the
// base-ui portal (which does not lay out meaningfully under jsdom).
function findItems(node: ReactNode): ContextMenuItem[] | null {
  if (!node || typeof node !== 'object') return null
  if (Array.isArray(node)) {
    for (const child of node) {
      const found = findItems(child)
      if (found) return found
    }
    return null
  }
  if (!isValidElement(node)) return null
  const props = node.props as { items?: unknown; children?: ReactNode }
  if (Array.isArray(props.items)) return props.items as ContextMenuItem[]
  return findItems(props.children ?? null)
}

const mouseEvent = { preventDefault() {}, stopPropagation() {}, pageX: 10, pageY: 10 }

function openMenu(options: ReturnType<typeof baseOptions>, path: string, isDir: boolean) {
  const { result } = renderHook(() => useFileExplorerContextMenu(options))
  act(() => {
    result.current.handleContextMenu(mouseEvent as unknown as React.MouseEvent, path, isDir)
  })
  const items = findItems(result.current.contextMenuElement) ?? []
  return { items, labels: items.map((i) => i.label) }
}

function click(items: ContextMenuItem[], label: string) {
  const item = items.find((i) => i.label === label)
  if (!item) throw new Error(`menu item not found: ${label}`)
  act(() => item.onClick())
}

describe('useFileExplorerContextMenu — newly-rendered items', () => {
  it('renders Upload File / Duplicate / Reveal in Finder on a non-root directory', () => {
    const { labels } = openMenu(baseOptions(), 'api', true)
    expect(labels).toContain('Upload File')
    expect(labels).toContain('Duplicate')
    expect(labels).toContain('Reveal in Finder')
    // Not the workspace root, so no workspace-level affordances.
    expect(labels).not.toContain('Add Folder to Workspace')
    expect(labels).not.toContain('Remove From Workspace')
  })

  it('gates Upload File to directories (hidden on files) but still shows Duplicate/Reveal', () => {
    const { labels } = openMenu(baseOptions(), 'api/main.go', false)
    expect(labels).not.toContain('Upload File')
    expect(labels).not.toContain('Add Folder to Workspace')
    expect(labels).toContain('Duplicate')
    expect(labels).toContain('Reveal in Finder')
    expect(labels).toContain('Rename')
    expect(labels).toContain('Delete')
  })

  it('shows Add Folder to Workspace only on the workspace root, where Duplicate/Rename are hidden', () => {
    const options = { ...baseOptions(), isWorkspaceRootPath: (p: string) => p === ROOT }
    const { labels } = openMenu(options, ROOT, true)
    expect(labels).toContain('Add Folder to Workspace')
    expect(labels).toContain('Upload File')
    // Root hides file-management actions.
    expect(labels).not.toContain('Duplicate')
    expect(labels).not.toContain('Rename')
    expect(labels).not.toContain('Delete')
  })

  it('shows Remove From Workspace only when canRemoveWorkspaceRootPath allows it', () => {
    const removable = openMenu(
      { ...baseOptions(), canRemoveWorkspaceRootPath: (p: string) => p === 'extra-root' },
      'extra-root',
      true,
    )
    expect(removable.labels).toContain('Remove From Workspace')

    const notRemovable = openMenu(baseOptions(), 'extra-root', true)
    expect(notRemovable.labels).not.toContain('Remove From Workspace')
  })

  it('passes the right path args to each handler', () => {
    const options = {
      ...baseOptions(),
      isWorkspaceRootPath: (p: string) => p === ROOT,
      canRemoveWorkspaceRootPath: (p: string) => p === 'extra-root',
    }

    // File: Duplicate + Reveal carry the node path; Reveal joins to absolute.
    const file = openMenu(options, 'api/main.go', false)
    click(file.items, 'Duplicate')
    expect(options.onDuplicatePath).toHaveBeenCalledWith('api/main.go')
    click(file.items, 'Reveal in Finder')
    expect(options.onRevealInFinder).toHaveBeenCalledWith(joinPath(ROOT, 'api/main.go'))

    // Non-root directory: Upload targets that directory.
    const dir = openMenu(options, 'api', true)
    click(dir.items, 'Upload File')
    expect(options.onUploadFile).toHaveBeenCalledWith('api')

    // Removable root: Remove passes its own path.
    const extra = openMenu(options, 'extra-root', true)
    click(extra.items, 'Remove From Workspace')
    expect(options.onRemoveFolderFromWorkspace).toHaveBeenCalledWith('extra-root')

    // Workspace root: Upload normalises to '' and Add Folder fires path-less.
    const root = openMenu(options, ROOT, true)
    click(root.items, 'Upload File')
    expect(options.onUploadFile).toHaveBeenCalledWith('')
    click(root.items, 'Add Folder to Workspace')
    expect(options.onAddFolderToWorkspace).toHaveBeenCalledTimes(1)
  })
})

describe('useFileExplorerContextMenu — locked workspace gating', () => {
  const MUTATION_LABELS = [
    'New File',
    'New Folder',
    'Upload File',
    'Duplicate',
    'Rename',
    'Delete',
    'Cut',
  ]
  const READ_ONLY_LABELS = ['Copy Path', 'Copy Relative Path', 'Reveal in Finder', 'Copy']

  it('hides every mutation item on a file while keeping read-only items (locked)', () => {
    const { labels } = openMenu({ ...baseOptions(), isLocked: true }, 'api/main.go', false)
    for (const label of MUTATION_LABELS) expect(labels).not.toContain(label)
    // Read-only items and the file-open action stay reachable.
    expect(labels).toContain('Open')
    expect(labels).toContain('Copy Content')
    for (const label of READ_ONLY_LABELS) expect(labels).toContain(label)
  })

  it('hides directory mutation items but keeps Refresh / Open All / Open in Terminal (locked)', () => {
    const { labels } = openMenu({ ...baseOptions(), isLocked: true }, 'api', true)
    expect(labels).not.toContain('New File')
    expect(labels).not.toContain('New Folder')
    expect(labels).not.toContain('Upload File')
    // Read-only directory affordances remain.
    expect(labels).toContain('Refresh')
    expect(labels).toContain('Open All Files')
    expect(labels).toContain('Collapse All')
    expect(labels).toContain('Open in Terminal')
  })

  it('hides env-template creation on a locked .env file', () => {
    // Unlocked, the .env file offers env-template creation…
    const unlocked = openMenu({ ...baseOptions(), isLocked: false }, '.env', false)
    expect(unlocked.labels).toContain('Create .env.example')
    // …locked, that mutation is gone but read-only actions stay.
    const locked = openMenu({ ...baseOptions(), isLocked: true }, '.env', false)
    expect(locked.labels).not.toContain('Create .env.example')
    expect(locked.labels).toContain('Open')
    expect(locked.labels).toContain('Copy Path')
  })

  it('unlocked workspace is unchanged — mutation items present', () => {
    // File-context mutations (New File/Folder + Upload are directory-only).
    const { labels } = openMenu({ ...baseOptions(), isLocked: false }, 'api/main.go', false)
    for (const label of ['Duplicate', 'Rename', 'Delete', 'Cut']) {
      expect(labels).toContain(label)
    }
    // Directory-context mutations.
    const dir = openMenu({ ...baseOptions(), isLocked: false }, 'api', true)
    expect(dir.labels).toContain('Upload File')
    expect(dir.labels).toContain('New File')
    expect(dir.labels).toContain('New Folder')
  })

  it('defaults to unlocked when isLocked is omitted', () => {
    const { labels } = openMenu(baseOptions(), 'api/main.go', false)
    expect(labels).toContain('Rename')
    expect(labels).toContain('Delete')
    expect(labels).toContain('Cut')
  })
})

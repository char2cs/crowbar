/**
 * Contract pins for the folder row.
 *
 * A folder is a new row TYPE in a tree that already had three, and the thing
 * worth pinning is that it did not get to invent its own conventions: same
 * ROW_BASE chrome, same 16px leading-glyph box, same collapse persistence, same
 * treeitem/group semantics. The glyph box in particular is load-bearing — folders
 * and branch rows interleave constantly, so a box that differed by even 2px would
 * put a visible wobble down the sidebar's left edge.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, within, cleanup } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
}))

const actions = vi.hoisted(() => ({
  creatingChildOf: null as { repoId: string; parentId: string } | null,
  startCreating: vi.fn(),
  confirmCreate: vi.fn(),
  cancelCreate: vi.fn(),
  renamingId: null as string | null,
  startRenaming: vi.fn(),
  confirmRename: vi.fn(),
  cancelRename: vi.fn(),
  onPointerDownDrag: vi.fn(),
  pendingCreates: new Map<
    string,
    { repoId: string; parentId: string; branch: string; error?: string }
  >(),
  clearPendingCreate: vi.fn(),
}))
const drag = vi.hoisted(() => ({
  draggingWs: null,
  draggingIds: new Set<string>(),
  dropTarget: null,
  movingIds: new Set<string>(),
}))

vi.mock('@/components/layout/workspace-tree-context', () => ({
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

import { FolderRow } from '@/components/layout/folder-row'
import { ROW_BASE, ROW_GLYPH_BOX } from '@/components/layout/workspace-row-base'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  buildSidebarTree,
  EMPTY_REPO_TREE,
  type SidebarTreeNode,
} from '@/components/layout/workspace-tree-utils'

/** A folder holding one workspace, built through the real tree builder. */
function folderNode(childCount = 1): Extract<SidebarTreeNode, { kind: 'folder' }> {
  const workspaces = Array.from({ length: childCount }, (_, i) => ({
    id: `w${i}`,
    branch: `feature/${i}`,
    folderId: 'f1',
    age: 'now',
  }))
  const roots = buildSidebarTree(workspaces, [{ id: 'f1', name: 'spikes' }])
  return roots.find((n) => n.id === 'f1') as Extract<SidebarTreeNode, { kind: 'folder' }>
}

function renderFolder(node = folderNode()) {
  return render(
    <FolderRow
      node={node}
      depth={0}
      repoId="r1"
      projectId="p1"
      activeWorkspaceId=""
      onWorkspaceClick={() => {}}
      tree={EMPTY_REPO_TREE}
    />,
  )
}

/**
 * The folder row itself. Its children hang off a SIBLING `role="group"`, not off
 * the row element, so the first treeitem is always the folder and `within` it
 * reaches only the folder's own controls.
 */
const folderRow = () => screen.getAllByRole('treeitem')[0]

/** The outline the leading glyph is currently drawn from. */
const glyphPath = () => folderRow().querySelector('svg > path')!.getAttribute('d')

beforeEach(() => {
  vi.clearAllMocks()
  actions.renamingId = null
  useSidebarStore.setState({
    repos: [{ id: 'r1', name: 'repo', workspaces: [] }] as never,
    collapsedWorkspaces: new Set<string>(),
  })
})

describe('FolderRow — tree contract', () => {
  it('is a treeitem carrying its own aria-expanded', () => {
    renderFolder()
    expect(folderRow().textContent).toContain('spikes')
    expect(folderRow()).toHaveAttribute('aria-expanded', 'true')
  })

  it('reports aria-expanded=false once collapsed', () => {
    useSidebarStore.setState({ collapsedWorkspaces: new Set(['f1']) })
    renderFolder()
    expect(folderRow()).toHaveAttribute('aria-expanded', 'false')
  })

  it('wraps its children in a fresh role="group"', () => {
    renderFolder()
    expect(screen.getByRole('group').textContent).toContain('feature/0')
  })

  it('renders no group at all when collapsed', () => {
    useSidebarStore.setState({ collapsedWorkspaces: new Set(['f1']) })
    renderFolder()
    expect(screen.queryByRole('group')).toBeNull()
    expect(screen.queryByText('feature/0')).toBeNull()
  })

  it('keeps its own controls reachable as real buttons', () => {
    // The whole point of treeitem over button: a presentational-children role
    // would erase these.
    renderFolder()
    const own = within(folderRow())
    expect(own.getByLabelText('Add child workspace')).toBeInTheDocument()
    expect(own.getByLabelText('Collapse')).toBeInTheDocument()
  })
})

describe('FolderRow — chrome', () => {
  it('is built from ROW_BASE rather than hand-rolled styles', () => {
    renderFolder()
    const classes = folderRow().className.split(/\s+/)
    for (const shared of ROW_BASE.trim().split(/\s+/)) {
      expect(classes, `folder row lost ${shared}`).toContain(shared)
    }
  })

  it('sets the name in the UI face, not the branch rows’ mono', () => {
    // Mono earns its place on a git ref, where the reader has to tell l from 1
    // and count the slashes. A folder name is prose someone typed, and the
    // change of face is the cheapest signal that this row is not a branch.
    renderFolder()
    const label = screen.getByText('spikes')
    expect(label.className).toContain('font-sans')
    expect(label.className.split(/\s+/)).not.toContain('font-mono')
  })

  it('keeps that face in the editor, so a rename does not change it mid-type', () => {
    actions.renamingId = 'f1'
    renderFolder()
    expect(screen.getByDisplayValue('spikes').className.split(/\s+/)).not.toContain('font-mono')
  })

  it('puts its glyph in the SAME 16px box every other leading glyph uses', () => {
    // One label position per level. An 18px box here would offset the folder's
    // label by 2px from the branch rows it sits between.
    renderFolder()
    const slot = folderRow().firstElementChild!
    const classes = slot.className.split(/\s+/)
    for (const shared of ROW_GLYPH_BOX.split(/\s+/)) {
      expect(classes, `folder glyph box lost ${shared}`).toContain(shared)
    }
  })

  it('takes the shared icon family’s own stroke weight, never an override', () => {
    // The rule workspace-row-base.ts spells out: the family's default IS the
    // sidebar's weight. Override it and the folder becomes the boldest mark in
    // a column of branch glyphs it is supposed to sit level with.
    renderFolder()
    const svg = folderRow().querySelector('svg')!
    expect(svg.getAttribute('viewBox')).toBe('0 0 24 24')
    expect(svg.getAttribute('stroke-width')).toBe('2')
  })

  it('swaps between two marks for open and closed rather than tweening one', () => {
    // At 16px an open folder is a few pixels of shear from a closed one, so a
    // tween spends its whole duration saying what the swap says at once.
    renderFolder()
    const openPath = glyphPath()
    const openClasses = folderRow().querySelector('svg')!.getAttribute('class') ?? ''
    expect(openClasses).not.toContain('transition')

    cleanup()
    useSidebarStore.setState({ collapsedWorkspaces: new Set(['f1']) })
    renderFolder()

    expect(glyphPath()).not.toBe(openPath)
  })
})

describe('FolderRow — behaviour', () => {
  it('toggles on a row click — a folder has nowhere to open', () => {
    renderFolder()
    fireEvent.click(folderRow())
    expect(useSidebarStore.getState().collapsedWorkspaces.has('f1')).toBe(true)
  })

  it('toggles from the chevron without the click reaching the row twice', () => {
    renderFolder()
    fireEvent.click(within(folderRow()).getByLabelText('Collapse'))
    expect(useSidebarStore.getState().collapsedWorkspaces.has('f1')).toBe(true)
  })

  it('offers no chevron when it holds nothing', () => {
    renderFolder(folderNode(0))
    expect(screen.queryByLabelText('Collapse')).toBeNull()
    expect(screen.queryByLabelText('Expand')).toBeNull()
    // ...but the "+" is still there, because an empty folder is exactly the one
    // you want to put something in.
    expect(screen.getByLabelText('Add child workspace')).toBeInTheDocument()
  })

  it('starts a create parented to the FOLDER, not to a workspace', () => {
    renderFolder()
    fireEvent.click(within(folderRow()).getByLabelText('Add child workspace'))
    expect(actions.startCreating).toHaveBeenCalledWith('r1', 'f1')
  })

  it('opens itself first when a create starts on a collapsed folder', () => {
    useSidebarStore.setState({ collapsedWorkspaces: new Set(['f1']) })
    renderFolder()
    fireEvent.click(screen.getByLabelText('Add child workspace'))
    expect(useSidebarStore.getState().collapsedWorkspaces.has('f1')).toBe(false)
  })

  it('carries the drop-target attribute the drag scan looks for', () => {
    renderFolder()
    expect(folderRow()).toHaveAttribute('data-folder-drop', 'f1')
  })
})

/**
 * A folder is created before it is named — grouping rows makes one on the spot —
 * so the row has to be able to say what it is called afterwards. It offers the
 * branch row's gesture, through the branch row's editor and the tree's one
 * `renamingId`: a folder is not a locked row, and nothing about it justifies a
 * second rename path.
 */
describe('FolderRow — rename', () => {
  it('opens the editor when the name is double-clicked', () => {
    renderFolder()

    fireEvent.doubleClick(screen.getByText('spikes'))

    expect(actions.startRenaming).toHaveBeenCalledWith('f1')
  })

  it('swaps the name for the shared inline editor while it is being typed', () => {
    actions.renamingId = 'f1'
    renderFolder()

    expect(screen.getByDisplayValue('spikes')).toBeInTheDocument()
    expect(screen.queryByText('spikes')).toBeNull()
  })

  it('cannot be picked up or dropped onto mid-rename', () => {
    // Same rule the branch row keeps: a row whose name is being typed is not a
    // drag handle, and it is not a slot for someone else to land in either.
    actions.renamingId = 'f1'
    renderFolder()

    expect(folderRow()).not.toHaveAttribute('data-folder-drop')
    fireEvent.pointerDown(folderRow())
    expect(actions.onPointerDownDrag).not.toHaveBeenCalled()
  })

  it('does not fold itself when a click lands in the editor', () => {
    actions.renamingId = 'f1'
    renderFolder()

    fireEvent.click(folderRow())

    expect(useSidebarStore.getState().collapsedWorkspaces.has('f1')).toBe(false)
  })
})

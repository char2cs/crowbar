/**
 * Contract pins for the workspace row's own chrome.
 *
 * Two rules, both of which look like styling and are not:
 *
 *  1. The add-child "+" keeps a stable flex slot at rest. Changing its display
 *     on hover resizes the label and forces layout for every crossed row, so
 *     visibility and pointer events alone change during interaction.
 *  2. The change counts moved to a second line INSIDE the unchanged 36px row.
 *     If the active row grew to fit them, every row below it would shift each
 *     time you switched workspaces.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

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

import { WorkspaceTreeItem } from '@/components/layout/workspace-tree-item'
import { ROW_BASE, ROW_GLYPH_BOX } from '@/components/layout/workspace-row-base'
import { useSidebarStore, type Workspace } from '@/lib/store/sidebar'
import {
  buildSidebarTree,
  EMPTY_REPO_TREE,
  type SidebarTreeNode,
} from '@/components/layout/workspace-tree-utils'

function node(over: Partial<Workspace> = {}): Extract<SidebarTreeNode, { kind: 'workspace' }> {
  const roots = buildSidebarTree([{ id: 'w1', branch: 'feature/x', age: 'now', ...over }])
  return roots[0] as Extract<SidebarTreeNode, { kind: 'workspace' }>
}

function renderRow(over: Partial<Workspace> = {}, activeWorkspaceId = '') {
  return render(
    <WorkspaceTreeItem
      node={node(over)}
      depth={0}
      repoId="r1"
      projectId="p1"
      activeWorkspaceId={activeWorkspaceId}
      onWorkspaceClick={() => {}}
      tree={EMPTY_REPO_TREE}
    />,
  )
}

const row = () => screen.getAllByRole('treeitem')[0]

beforeEach(() => {
  vi.clearAllMocks()
  actions.creatingChildOf = null
  actions.pendingCreates = new Map()
  useSidebarStore.setState({
    repos: [{ id: 'r1', name: 'repo', workspaces: [] }] as never,
    collapsedWorkspaces: new Set<string>(),
  })
})

describe('the add-child "+" keeps row geometry stable', () => {
  it('stays in layout while hidden and becomes interactive on hover or focus', () => {
    renderRow()

    const classes = screen.getByLabelText('Add child workspace').className.split(/\s+/)
    expect(classes).toContain('inline-flex')
    expect(classes).toContain('invisible')
    expect(classes).toContain('pointer-events-none')
    expect(classes).toContain('group-hover:visible')
    expect(classes).toContain('group-hover:pointer-events-auto')
    expect(classes).toContain('group-focus-within:visible')
    expect(classes).toContain('group-focus-within:pointer-events-auto')
    expect(classes).not.toContain('hidden')
    expect(classes).not.toContain('group-hover:inline-flex')
  })

  it('changes visibility and hit testing without changing display', () => {
    renderRow()
    const variants = screen
      .getByLabelText('Add child workspace')
      .className.split(/\s+/)
      .filter((className) => className.startsWith('group-hover:'))
    expect(variants).toEqual(
      expect.arrayContaining(['group-hover:visible', 'group-hover:pointer-events-auto']),
    )
    expect(variants.some((className) => /(?:hidden|flex|inline|block)$/.test(className))).toBe(
      false,
    )
  })

  it('needs the row to be a `group` for the hover rule to resolve', () => {
    renderRow()
    expect(row().className.split(/\s+/)).toContain('group')
  })

  it('is still offered on a row that already has children', () => {
    const parent = buildSidebarTree([
      { id: 'w1', branch: 'feature/x', age: 'now' },
      { id: 'w2', branch: 'feature/y', parentId: 'w1', age: 'now' },
    ])[0] as Extract<SidebarTreeNode, { kind: 'workspace' }>
    render(
      <WorkspaceTreeItem
        node={parent}
        depth={0}
        repoId="r1"
        projectId="p1"
        activeWorkspaceId=""
        onWorkspaceClick={() => {}}
        tree={EMPTY_REPO_TREE}
      />,
    )

    // Both controls, in that order: "+" then the collapse chevron.
    const controls = Array.from(screen.getAllByRole('treeitem')[0].querySelectorAll('button')).map(
      (b) => b.getAttribute('aria-label'),
    )
    expect(controls).toEqual(['Add child workspace', 'Collapse'])
  })
})

describe('change counts render as a second line, not a second column', () => {
  it('puts the counts under the branch name in the same label column', () => {
    renderRow({ added: 12, deleted: 3 }, 'w1')

    const label = screen.getByText('feature/x').parentElement!
    expect(label.className).toContain('flex-col')
    expect(label.textContent).toContain('+12')
    expect(label.textContent).toContain('-3')
  })

  it('leaves the row at its unchanged 36px height', () => {
    // h-9 is a FIXED 36px, so the second line cannot push the row taller. If it
    // ever became a min-height, every row below the active one would shift on
    // each workspace switch.
    renderRow({ added: 12, deleted: 3 }, 'w1')

    const classes = row().className.split(/\s+/)
    expect(classes).toContain('h-9')
    expect(ROW_BASE).toContain('h-9')
    expect(classes.filter((c) => /^min-h-|^h-(?!9$)/.test(c))).toEqual([])
  })

  it('uses the muted TOKEN and tabular figures, never a faded foreground', () => {
    renderRow({ added: 12, deleted: 3 }, 'w1')

    // The LINE is muted; the figures on it are not (see below).
    const line = screen.getByText(/\+12/).parentElement!
    expect(line.className).toContain('text-muted-foreground')
    expect(line.className).toContain('tabular-nums')
    expect(line.className).toContain('text-[10.5px]')
    // A transparency composites differently over the sidebar glass, the hover
    // accent and the raised active surface — that drift is why the token exists.
    expect(line.className).not.toMatch(/text-foreground\//)
  })

  it('keeps the counts green and red, so a + and a - are not the only difference', () => {
    // Muting the whole line would leave an insertion and a deletion separated
    // by one glyph a pixel wide at 10.5px. The line is muted; the figures keep
    // the colour they have always had.
    renderRow({ added: 12, deleted: 3 }, 'w1')

    // Tokens, not palette classes: the light theme takes both down so they read
    // against a light surface, which a bare `text-green-300` cannot express.
    expect(screen.getByText(/\+12/).className).toContain('text-sidebar-count-added')
    expect(screen.getByText(/-3/).className).toContain('text-sidebar-count-deleted')
  })

  it('keeps the name truncating — the counts must not cost it its min-w-0', () => {
    renderRow({ added: 12, deleted: 3 }, 'w1')

    const label = screen.getByText('feature/x').parentElement!
    expect(label.className).toContain('min-w-0')
    expect(screen.getByText('feature/x').className).toContain('truncate')
  })

  it('shows no second line on an inactive row', () => {
    renderRow({ added: 12, deleted: 3 }, 'w-other')
    expect(screen.queryByText(/\+12/)).toBeNull()
  })

  it('shows no second line when nothing changed', () => {
    renderRow({ added: 0, deleted: 0 }, 'w1')
    expect(screen.getByText('feature/x').parentElement!.childElementCount).toBe(1)
  })
})

describe('one label position per level', () => {
  it('gives the status glyph the shared 16px box', () => {
    renderRow()
    const slot = row().firstElementChild!
    for (const shared of ROW_GLYPH_BOX.split(/\s+/)) {
      expect(slot.className.split(/\s+/), `workspace glyph box lost ${shared}`).toContain(shared)
    }
  })
})

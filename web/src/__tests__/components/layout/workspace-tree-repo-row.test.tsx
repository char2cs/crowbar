/**
 * Contract pins for the repo header row and the subtree hanging off it.
 *
 * These are the parts the rest of the sidebar reaches into from the outside and
 * that nothing else asserted: the drag-and-drop system finds the row by its
 * `data-repo-drop` attribute and rings it from the resolved drop, the keyboard
 * path only fires when the row ITSELF is the event target, the rename guard has
 * to keep a click off the navigation, and the repo-root create/pending rows sit
 * at one indent step (14px). Behaviour-preserving refactors of the row must
 * leave every one of these alone.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

const navigateFn = vi.hoisted(() => vi.fn())
const router = vi.hoisted(() => ({ pathname: '' }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateFn,
  useRouterState: () => router.pathname,
  useRouter: () => ({ state: { location: { pathname: router.pathname } } }),
  useMatch: () => null,
}))

// Drive the tree context directly: the test needs the drag hover parked on this
// repo, and an in-flight / in-progress create staged, without a real pointer
// drag or a network round trip.
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
  dropTarget: null as { kind: string; id: string; mode: string } | null,
  movingIds: new Set<string>(),
}))

vi.mock('@/components/layout/workspace-tree-context', () => ({
  WorkspaceTreeProvider: ({ children }: { children: React.ReactNode }) => children,
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, renameRepo: vi.fn(() => Promise.resolve()) }
})

import { WorkspaceTree } from '@/components/layout/workspace-tree'
import { idle } from '@/lib/loadable'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [{ id: 'ws1', branch: 'feature/x', status: 'new', age: '1d' }],
  ...over,
})

const OPEN_REPO_PARAMS = {
  to: '/ide/$projectId/$repoId/$wsId',
  params: { projectId: 'p1', repoId: 'r1', wsId: 'w-default' },
}

beforeEach(() => {
  navigateFn.mockClear()
  router.pathname = ''
  actions.creatingChildOf = null
  actions.pendingCreates = new Map()
  drag.dropTarget = null
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useSidebarStore.setState({ repos: [repo()], collapsedRepos: new Set<string>() })
  useSidebarSelectionStore.setState({ kept: new Set<string>() })
})

describe('repo header row: drag-and-drop surface', () => {
  // findDrop() in drop-target-dom walks elementsFromPoint looking for exactly
  // this attribute — drop it and every drop onto a repo is a no-op.
  it('carries data-repo-drop so the drop-target scan can find it', () => {
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar')).toHaveAttribute('data-repo-drop', 'r1')
  })

  // The same-repo rule compares a workspace's scope against the target's, so a
  // repo header with no scope of its own refuses every drop onto it.
  it('publishes its own id as the drop scope', () => {
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar')).toHaveAttribute('data-drop-repo', 'r1')
  })

  it('fills the row while a drop would land INSIDE it', () => {
    drag.dropTarget = { kind: 'repo', id: 'r1', mode: 'into' }
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar').className).toContain('bg-sidebar-drop-nest')
  })

  it('does not fill the row when another repo is the drop target', () => {
    drag.dropTarget = { kind: 'repo', id: 'r2', mode: 'into' }
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar').className).not.toContain('bg-sidebar-drop-nest')
  })
})

describe('repo header row: keyboard activation', () => {
  it('opens the repo home on Enter', () => {
    render(<WorkspaceTree />)

    fireEvent.keyDown(screen.getByLabelText('Open crowbar'), { key: 'Enter' })

    expect(navigateFn).toHaveBeenCalledWith(OPEN_REPO_PARAMS)
  })

  it('opens the repo home on Space', () => {
    render(<WorkspaceTree />)

    fireEvent.keyDown(screen.getByLabelText('Open crowbar'), { key: ' ' })

    expect(navigateFn).toHaveBeenCalledWith(OPEN_REPO_PARAMS)
  })

  // The `e.target === e.currentTarget` guard: a key pressed on a nested control
  // bubbles to the row, and without the guard tabbing to "Import branches" and
  // hitting Enter would ALSO navigate away behind the modal it just opened.
  it('ignores an Enter that bubbled up from a nested control', () => {
    render(<WorkspaceTree />)

    fireEvent.keyDown(screen.getByLabelText('Import branches'), { key: 'Enter', bubbles: true })

    expect(navigateFn).not.toHaveBeenCalled()
  })
})

describe('repo header row: rename guard', () => {
  // `fireEvent.click`, not `user.click`: a full pointer sequence blurs the
  // rename input first, which COMMITS the rename and clears the renaming id
  // before the click handler runs — so the row navigates. That is the row's
  // behaviour today and this test is not about it. A bare click isolates the
  // `renamingRepoId === repo.id` guard itself, which is what stops the row
  // navigating out from under a rename in progress.
  it('does not navigate on a click while its name is being renamed', () => {
    render(<WorkspaceTree />)

    fireEvent.doubleClick(screen.getByText('crowbar'))
    expect(screen.getByDisplayValue('crowbar')).toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Open crowbar'))

    expect(navigateFn).not.toHaveBeenCalled()
  })

  it('ignores an Enter on the row while renaming', () => {
    render(<WorkspaceTree />)

    fireEvent.doubleClick(screen.getByText('crowbar'))
    fireEvent.keyDown(screen.getByLabelText('Open crowbar'), { key: 'Enter' })

    expect(navigateFn).not.toHaveBeenCalled()
  })
})

describe('repo header row: row chrome', () => {
  it('spends no row width on a "- default" hint', () => {
    render(<WorkspaceTree />)

    // The hint used to sit between the name and the trailing buttons, so a long
    // repo name truncated earlier than it had to. The row's only destination IS
    // the repo home, which made the label redundant with where you already are.
    expect(screen.queryByText('- default')).toBeNull()
  })

  it('orders its trailing actions like every other workspace row', () => {
    render(<WorkspaceTree />)

    const row = screen.getByLabelText('Open crowbar')
    const labels = [...row.querySelectorAll('button')].map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual([
      // Leading: the avatar doubles as the icon-editing trigger.
      'Edit crowbar icon',
      'Add child workspace',
      // The repo-ONLY action comes after everything this row shares with the
      // workspace rows, and before the disclosure that closes every row.
      'Import branches',
      'Collapse repo',
    ])
  })

  it('leads the trailing cluster with fold-away, as a folder row does', () => {
    // Only a FOLDED row still showing kept rows grows this button, so the plain
    // order test above never reaches it. Fold-away is the one control here that
    // appears because of what the row is DOING rather than what it is, so it
    // reads first and never shifts the three fixed slots behind it.
    useSidebarStore.setState({ repos: [repo()], collapsedRepos: new Set(['r1']) })
    useSidebarSelectionStore.setState({ kept: new Set(['ws1']) })
    render(<WorkspaceTree />)

    const row = screen.getByLabelText('Open crowbar')
    const labels = [...row.querySelectorAll('button')].map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual([
      'Edit crowbar icon',
      'Fold away the rows crowbar is holding',
      'Add child workspace',
      'Import branches',
      'Expand repo',
    ])
  })

  it('marks the open row so its actions can stay on screen', () => {
    // ROW_ACTIVE is a class string, which no CSS variant can select — the
    // reveal-when-active rule in ROW_SUB_ACTION_HOVER reads this attribute
    // instead. jsdom applies no Tailwind, so the attribute IS the contract here;
    // the CSS half was verified against the running app.
    router.pathname = '/ide/p1/r1/w-default'
    render(<WorkspaceTree />)

    const row = screen.getByLabelText('Open crowbar')
    expect(row).toHaveAttribute('data-active')
    expect(row.querySelector('button[aria-label="Add child workspace"]')!.className).toContain(
      'group-data-[active]:inline-flex',
    )
  })

  it('leaves the marker off a row that is not the open one', () => {
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar')).not.toHaveAttribute('data-active')
  })

  it("reveals add-child on hover with the branch rows' own reveal", () => {
    render(<WorkspaceTree />)

    const adds = screen
      .getAllByLabelText('Add child workspace')
      .map((b) => (b as HTMLElement).className)
    // This row IS a workspace (the repo's default), so it had no business
    // revealing its controls differently from the rows beneath it. Comparing
    // against a real branch row's button keeps the two in lockstep rather than
    // asserting a class list this test would have to be taught twice.
    expect(adds.length).toBeGreaterThan(1)
    expect(new Set(adds).size).toBe(1)
    expect(adds[0]).toContain('hidden')
    expect(adds[0]).toContain('group-hover:inline-flex')
  })

  it('wears the shared active-row surface when the repo home is the open workspace', () => {
    router.pathname = '/ide/p1/r1/w-default'
    render(<WorkspaceTree />)

    const className = screen.getByLabelText('Open crowbar').className
    expect(className).toContain('bg-background')
    expect(className).toContain('shadow-xs')
  })

  it('wears the inactive-row surface otherwise', () => {
    router.pathname = '/ide/p1/r1/ws1'
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open crowbar').className).toContain('hover:bg-accent')
  })
})

// A row's indent is `margin-inline-start` rather than padding, because it MOVES:
// a row kept through a collapse is re-drawn one step under whatever is holding
// it, and the transition needs a property on the box that shifts.
describe('repo-root create rows: indentation', () => {
  it('indents the inline create input one step (14px) under the repo header', () => {
    actions.creatingChildOf = { repoId: 'r1', parentId: 'w-default' }
    render(<WorkspaceTree />)

    const input = screen.getByPlaceholderText('branch-name, or name/ for a folder')
    // input → WorkspaceInlineInput wrapper → ROW_BASE row → indented container
    expect(input.parentElement?.parentElement?.parentElement).toHaveStyle({
      marginInlineStart: '14px',
    })
  })

  it('indents an in-flight pending create row one step (14px)', () => {
    actions.pendingCreates = new Map([
      ['t1', { repoId: 'r1', parentId: 'w-default', branch: 'feature/pending' }],
    ])
    render(<WorkspaceTree />)

    const label = screen.getByText('feature/pending')
    // label → ROW_BASE row → indented container
    expect(label.parentElement?.parentElement).toHaveStyle({ marginInlineStart: '14px' })
  })

  it('does not render a pending create belonging to another repo', () => {
    actions.pendingCreates = new Map([
      ['t1', { repoId: 'r2', parentId: 'w-default', branch: 'feature/elsewhere' }],
    ])
    render(<WorkspaceTree />)

    expect(screen.queryByText('feature/elsewhere')).toBeNull()
  })

  it('does not render a pending create forked from a non-default parent here', () => {
    actions.pendingCreates = new Map([
      ['t1', { repoId: 'r1', parentId: 'ws1', branch: 'feature/nested' }],
    ])
    render(<WorkspaceTree />)

    // It belongs under the ws1 row (rendered by WorkspaceTreeItem), not at the
    // repo root — the repo-root filter is `parentId === defaultWorkspaceId`.
    const label = screen.queryByText('feature/nested')
    expect(label?.parentElement?.parentElement).not.toHaveStyle({ marginInlineStart: '14px' })
  })
})

// The hover-only controls take NO space while hidden, so the row's name measures
// against the whole row. They used to reserve their slot permanently, which
// truncated a branch name at the same place on a row showing no controls at all.
//
// They are not floated out of flow to achieve that either: that fixed the idle
// width and then painted the button on top of the very text it had made room
// for. Returning to the flex flow is what makes the LABEL shrink instead.
describe('repo header row: trailing controls', () => {
  it('gives the name its width back and never paints over it', () => {
    render(<WorkspaceTree />)
    const row = screen.getByLabelText('Open crowbar')

    const add = row.querySelector('button[aria-label="Add child workspace"]')!
    expect(add.className).toContain('hidden')
    expect(add.className).toContain('group-hover:inline-flex')
    expect(add.className).not.toContain('invisible')
    expect(add.className).not.toContain('absolute')
    expect(add.parentElement!.className).not.toContain('absolute')

    // The always-visible controls never left the flow to begin with.
    const disclosure = row.querySelector('button[aria-label="Collapse repo"]')!
    expect(disclosure.className).not.toContain('absolute')
  })
})

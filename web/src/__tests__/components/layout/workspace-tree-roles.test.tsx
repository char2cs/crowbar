/**
 * Contract pin: the sidebar's tree semantics.
 *
 * These rows were `role="button"` once. A `button` takes PRESENTATIONAL
 * children, so every nested control inside a row — import branches, add child,
 * expand — was stripped of its own semantics for assistive tech. The fix was to
 * declare what these rows actually are: `role="tree"` on the container,
 * `role="treeitem"` on every row, `role="group"` on every children wrapper.
 * Unlike `button`, `treeitem` permits interactive descendants.
 *
 * That contract lived only in a comment in workspace-tree.tsx and in the DOM
 * itself — nothing asserted it, so a refactor could quietly undo the exact bug
 * it was written to fix. It is asserted here instead. A new row type (folders)
 * must join the same contract rather than inventing its own.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'parent', branch: 'develop', status: 'locked', age: '1d' },
    { id: 'child', branch: 'feature/x', parentId: 'parent', status: 'new', age: '1d' },
  ],
  folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
  ...over,
})

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

beforeEach(() => {
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('sidebar tree semantics', () => {
  it('declares a single labelled tree container', () => {
    render(<WorkspaceTree />)
    expect(screen.getByRole('tree', { name: 'Workspaces' })).toBeInTheDocument()
  })

  it('makes every project, repo, folder and workspace row a treeitem, never a button', () => {
    render(<WorkspaceTree />)

    const labels = screen
      .getAllByRole('treeitem')
      .map((row) => row.textContent ?? '')
      .join(' ')

    // Every row type in the tree, including the two Wave-4 additions (the
    // project row, which used to be a `role="button"` above the tree, and the
    // folder row, which is new).
    expect(labels).toContain('crowbar-project')
    expect(labels).toContain('crowbar')
    expect(labels).toContain('spikes')
    expect(labels).toContain('develop')
    expect(labels).toContain('feature/x')
  })

  it('leaves no `role="button"` row inside the tree', () => {
    // A tree's children must be treeitems or groups. `button` also takes
    // PRESENTATIONAL children, which is what erased the per-row controls the
    // first time round.
    render(<WorkspaceTree />)

    const tree = screen.getByRole('tree', { name: 'Workspaces' })
    expect(tree.querySelectorAll('[role="button"]')).toHaveLength(0)
  })

  it('keeps a row’s own controls reachable as buttons', () => {
    // This is the whole point of treeitem over button: a presentational-children
    // role would erase these.
    render(<WorkspaceTree />)
    expect(screen.getByLabelText('Import branches')).toBeInTheDocument()
    expect(screen.getAllByLabelText('Add child workspace').length).toBeGreaterThan(0)
  })

  it('wraps children in a group so depth is announced', () => {
    render(<WorkspaceTree />)
    const groups = screen.getAllByRole('group')
    expect(groups.length).toBeGreaterThan(0)
    // The repo's group holds the roots; the parent workspace's group holds its child.
    expect(groups.some((g) => (g.textContent ?? '').includes('feature/x'))).toBe(true)
  })

  it('reflects collapse state on the row that owns it', () => {
    render(<WorkspaceTree />)
    const expandable = screen
      .getAllByRole('treeitem')
      .filter((row) => row.hasAttribute('aria-expanded'))
    expect(expandable.length).toBeGreaterThan(0)
    for (const row of expandable) {
      expect(['true', 'false']).toContain(row.getAttribute('aria-expanded'))
    }
  })

  it('gives the folder row its OWN aria-expanded, not the repo’s', () => {
    useSidebarStore.setState({ collapsedWorkspaces: new Set(['f1']) })
    render(<WorkspaceTree />)

    const folderRow = screen
      .getAllByRole('treeitem')
      .find((row) => (row.textContent ?? '').includes('spikes'))!
    expect(folderRow).toHaveAttribute('aria-expanded', 'false')
    // ...and the repo above it is still open.
    const repoRow = screen.getByLabelText('Open crowbar')
    expect(repoRow).toHaveAttribute('aria-expanded', 'true')
  })

  it('puts a folder’s children in a fresh group of their own', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'parent', branch: 'develop', status: 'locked', age: '1d' },
            { id: 'inside', branch: 'feature/inside', folderId: 'f1', status: 'new', age: '1d' },
          ],
        }),
      ],
    })
    render(<WorkspaceTree />)

    const folderRow = screen
      .getAllByRole('treeitem')
      .find((row) => (row.textContent ?? '').includes('spikes'))!
    // The group holding the folder's child is a DESCENDANT of the folder row's
    // own subtree, not a sibling list the repo owns.
    const folderSubtree = folderRow.parentElement!.parentElement!
    const group = folderSubtree.querySelector('[role="group"]')
    expect(group?.textContent).toContain('feature/inside')
  })
})

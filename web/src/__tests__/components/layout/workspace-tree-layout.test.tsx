/**
 * Contract pins for the sidebar's layout after Wave 4.
 *
 * Every project lives in ONE scroller, separated by a rule, with its repos at
 * the project's own indent — the first indent step belongs to a repo's
 * workspaces, not to the repo row. One "New Project" row closes the list;
 * that row is what replaced the pushed project-switcher panel.
 *
 * Two structural things are pinned because breaking them is silent:
 *
 *  - The outer `flex flex-1 flex-col overflow-hidden` div must stay the SINGLE
 *    flex child of the carousel panel. sidebar-carousel.tsx computes scroll-snap
 *    offsets from the panel's own width, and a second child breaks that
 *    arithmetic in a way no assertion elsewhere would notice.
 *  - `rootsByRepo` is memoized. Its input is the whole `repos` array precisely
 *    so that folders and order — which live on the repo and on the workspace
 *    rows respectively — cannot change without the tree repainting.
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
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

const project = (id: string, name: string): Project => ({
  id,
  name,
  path: `/${id}`,
  lastActivity: new Date(0),
})

const repo = (id: string, projectId: string, over: Partial<Repo> = {}): Repo => ({
  id,
  projectId,
  name: `repo-${id}`,
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: `${id}-default`,
  workspaces: [{ id: `${id}-ws`, branch: `feature/${id}`, status: 'new', age: '1d' }],
  ...over,
})

beforeEach(() => {
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project('p1', 'alpha'), project('p2', 'beta')]) })
  useSidebarStore.setState({
    repos: [repo('r1', 'p1'), repo('r2', 'p2')],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('every project in one scroll', () => {
  it('renders a row for each project, both inside the single tree', () => {
    render(<WorkspaceTree />)

    const tree = screen.getByRole('tree', { name: 'Workspaces' })
    expect(tree.textContent).toContain('alpha')
    expect(tree.textContent).toContain('beta')
    expect(screen.getAllByRole('tree')).toHaveLength(1)
  })

  it('renders every project’s repos at once — nothing needs expanding first', () => {
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open repo-r1')).toBeInTheDocument()
    expect(screen.getByLabelText('Open repo-r2')).toBeInTheDocument()
  })

  it('separates the projects with an <hr>, and never leads with one', () => {
    render(<WorkspaceTree />)

    // Scoped to the TREE: the rule closing the list sits outside it, above the
    // New Project row, and is asserted with that row instead.
    const rules = screen.getByRole('tree').querySelectorAll('hr')
    // Two projects, one rule between them.
    expect(rules).toHaveLength(1)
    // Decorative: the rows either side already announce the section change.
    expect(rules[0]).toHaveAttribute('aria-hidden', 'true')
  })

  it('folds a project’s repos away without touching the other project', () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['p1']) })
    render(<WorkspaceTree />)

    expect(screen.queryByLabelText('Open repo-r1')).toBeNull()
    expect(screen.getByLabelText('Open repo-r2')).toBeInTheDocument()
    // The folded project keeps its own row — collapse is not removal.
    expect(screen.getByRole('tree').textContent).toContain('alpha')
  })

  it('keeps a repo whose ProjectDTO has not landed yet', () => {
    // A fresh import: the repo stream can beat /v0/projects. Dropping the rows
    // until the project row exists would blink the sidebar empty.
    useProjectDataStore.setState({ data: idle() })
    render(<WorkspaceTree />)

    expect(screen.getByLabelText('Open repo-r1')).toBeInTheDocument()
  })
})

describe('indentation', () => {
  it('puts repo rows at the project’s own indent, and workspaces one step in', () => {
    render(<WorkspaceTree />)

    const repoRow = screen.getByLabelText('Open repo-r1')
    const wsRow = screen
      .getAllByRole('treeitem')
      .find((r) => (r.textContent ?? '').includes('feature/r1'))!

    // The repo row is not inside any indent container...
    expect(repoRow.parentElement).not.toHaveStyle({ marginInlineStart: '14px' })
    // ...and its workspaces are, at exactly one step. `margin-inline-start`
    // rather than padding because the indent MOVES: a row kept through a
    // collapse is re-drawn one step under whatever is holding it.
    expect(wsRow.parentElement).toHaveStyle({ marginInlineStart: '14px' })
  })
})

describe('the list closes with one New Project row', () => {
  it('offers exactly one, separated from the last project by a rule', () => {
    render(<WorkspaceTree />)

    expect(screen.getAllByText('New Project')).toHaveLength(1)
    // The rule marks a bigger change of kind than the one between two project
    // sections: a section, then an action.
    const tree = screen.getByRole('tree')
    expect(tree.nextElementSibling?.tagName).toBe('HR')
    expect(tree.nextElementSibling?.nextElementSibling?.textContent).toBe('New Project')
  })

  it('sits OUTSIDE the tree — it is an action, not a node', () => {
    // A bare <button> among the treeitems would break the "N of M" position
    // announcement and claim a depth it does not have.
    render(<WorkspaceTree />)

    const tree = screen.getByRole('tree')
    expect(tree.textContent).not.toContain('New Project')
    expect(screen.getByText('New Project').closest('[role="tree"]')).toBeNull()
  })

  it('is offered even with no projects at all — and draws no rule above nothing', () => {
    useProjectDataStore.setState({ data: success([]) })
    useSidebarStore.setState({ repos: [] })
    render(<WorkspaceTree />)

    expect(screen.getByText('New Project')).toBeInTheDocument()
    // A rule under an empty list would hang off the top of the sidebar.
    expect(screen.getByRole('tree').nextElementSibling?.tagName).not.toBe('HR')
  })
})

describe('carousel contract', () => {
  it('keeps the outer flex column as the single root child', () => {
    const { container } = render(<WorkspaceTree />)

    // WorkspaceTreeProvider renders a fragment, so the tree's own root is the
    // first element — and the scroll-snap arithmetic needs it to be the only one
    // (the drag ghost only mounts mid-drag).
    const root = container.firstElementChild!
    expect(root.className).toBe('flex flex-1 flex-col overflow-hidden')
    expect(container.childElementCount).toBe(1)
  })
})

describe('rootsByRepo repaints on folders and order', () => {
  it('paints folder rows the builder produced from repo.folders', () => {
    useSidebarStore.setState({
      repos: [
        repo('r1', 'p1', {
          folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
          workspaces: [{ id: 'w1', branch: 'feature/inside', folderId: 'f1', age: '1d' }],
        }),
      ],
    })
    render(<WorkspaceTree />)

    expect(screen.getByRole('tree').textContent).toContain('spikes')
  })

  it('repaints when only `order` moves', () => {
    // The memo's input is the whole repos array, so a reorder — which changes
    // nothing but the workspaces' `order` — still lands.
    useSidebarStore.setState({
      repos: [
        repo('r1', 'p1', {
          workspaces: [
            { id: 'a', branch: 'feature/a', order: 0, age: '1d' },
            { id: 'b', branch: 'feature/b', order: 1, age: '1d' },
          ],
        }),
      ],
    })
    const { rerender } = render(<WorkspaceTree />)

    const order = () =>
      screen
        .getAllByRole('treeitem')
        .map((r) => r.textContent ?? '')
        .filter((t) => t.includes('feature/'))
        .map((t) => (t.includes('feature/a') ? 'a' : 'b'))

    expect(order()).toEqual(['a', 'b'])

    useSidebarStore.setState({
      repos: [
        repo('r1', 'p1', {
          workspaces: [
            { id: 'a', branch: 'feature/a', order: 1, age: '1d' },
            { id: 'b', branch: 'feature/b', order: 0, age: '1d' },
          ],
        }),
      ],
    })
    rerender(<WorkspaceTree />)

    expect(order()).toEqual(['b', 'a'])
  })

  it('repaints when only `folders` moves', () => {
    useSidebarStore.setState({
      repos: [repo('r1', 'p1', { workspaces: [] })],
    })
    const { rerender } = render(<WorkspaceTree />)
    expect(screen.getByRole('tree').textContent).not.toContain('spikes')

    useSidebarStore.setState({
      repos: [
        repo('r1', 'p1', {
          workspaces: [],
          folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
        }),
      ],
    })
    rerender(<WorkspaceTree />)

    expect(screen.getByRole('tree').textContent).toContain('spikes')
  })
})

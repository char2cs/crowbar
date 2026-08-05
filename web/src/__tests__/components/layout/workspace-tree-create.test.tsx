/**
 * The create flow: one input, two kinds of row.
 *
 * A trailing slash means folder — the same thing it means everywhere else a
 * path is typed — so there is no second button and no right-click-only path.
 * The pin that matters is that BOTH halves go through the same input and the
 * same commit, because a second entry point is how the two drift apart.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() => Promise.resolve()),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() => Promise.resolve()),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, postWorkspace: vi.fn(() => Promise.resolve()) }
})

import { createFolder } from '@/lib/api/sidebar-placement'
import { postWorkspace } from '@/lib/api'
import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { CREATE_ROW_PLACEHOLDER } from '@/components/layout/workspace-row-base'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  folders: [{ id: 'f-rev', repoId: 'r1', name: 'Reviewing', order: 0 }],
  workspaces: [{ id: 'w-develop', branch: 'develop', status: 'locked', age: '1d', order: 0 }],
})

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

const rowFor = (text: string) =>
  screen.getAllByRole('treeitem').find((r) => (r.textContent ?? '').includes(text))!

/** Open the create input on `row` and commit `value` from it. */
function createFrom(row: HTMLElement, value: string) {
  fireEvent.click(within(row).getByLabelText('Add child workspace'))
  const input = screen.getByPlaceholderText(CREATE_ROW_PLACEHOLDER)
  fireEvent.change(input, { target: { value } })
  fireEvent.keyDown(input, { key: 'Enter' })
}

beforeEach(() => {
  vi.clearAllMocks()
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

describe('the row’s "+" opens one input', () => {
  it('says what the trailing slash does, because nothing else can', () => {
    render(<WorkspaceTree />)
    fireEvent.click(within(rowFor('develop')).getByLabelText('Add child workspace'))

    expect(screen.getByPlaceholderText(CREATE_ROW_PLACEHOLDER)).toBeInTheDocument()
  })

  it('offers exactly one input, never a second button beside it', () => {
    render(<WorkspaceTree />)
    fireEvent.click(within(rowFor('develop')).getByLabelText('Add child workspace'))

    expect(screen.getAllByPlaceholderText(CREATE_ROW_PLACEHOLDER)).toHaveLength(1)
  })
})

describe('a trailing slash means folder', () => {
  it('creates a folder hanging off the row it was typed on', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('develop'), 'Reviewing/')

    expect(createFolder).toHaveBeenCalledWith('p1', 'r1', 'Reviewing', 'w-develop')
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  it('files a folder started from the repo header at the repo ROOT', () => {
    // The repo header's "+" carries repo home as its parent — that is what a
    // new branch forks from — but repo home is the header, not a row, so a
    // folder started there belongs at the root rather than under it.
    render(<WorkspaceTree />)

    createFrom(screen.getByLabelText('Open crowbar'), 'Spikes/')

    expect(createFolder).toHaveBeenCalledWith('p1', 'r1', 'Spikes', '')
  })

  it('creates a folder inside a folder', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('Reviewing'), 'Nested/')

    expect(createFolder).toHaveBeenCalledWith('p1', 'r1', 'Nested', 'f-rev')
  })

  it('takes the slash off the name', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('develop'), 'Long name here/')

    expect(createFolder).toHaveBeenCalledWith('p1', 'r1', 'Long name here', 'w-develop')
  })

  it('creates nothing from a bare slash', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('develop'), '/')

    expect(createFolder).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

describe('anything else is a workspace', () => {
  it('keeps the existing create path, optimistic row and all', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('develop'), 'feature/x')

    expect(postWorkspace).toHaveBeenCalledWith('p1', 'r1', 'feature/x', { parentId: 'w-develop' })
    expect(createFolder).not.toHaveBeenCalled()
    // The optimistic row is what the workspace half has that the folder half
    // does not: a folder answers synchronously, a worktree does not.
    expect(screen.getByText('feature/x')).toBeInTheDocument()
  })

  it('does not treat a mid-string slash as a folder', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('develop'), 'fix/import-pull')

    expect(postWorkspace).toHaveBeenCalledWith('p1', 'r1', 'fix/import-pull', {
      parentId: 'w-develop',
    })
    expect(createFolder).not.toHaveBeenCalled()
  })
})

// A folder has no branch, so it cannot be what a new branch forks from. The row
// the "+" was pressed on therefore answers two different questions depending on
// what it is, and sending a folder id as the fork parent is the create that
// could not resolve one.
describe('creating a workspace inside a folder', () => {
  it('sends the folder as placement and no fork parent at all', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('Reviewing'), 'feature/spike')

    expect(postWorkspace).toHaveBeenCalledWith('p1', 'r1', 'feature/spike', { folderId: 'f-rev' })
    expect(createFolder).not.toHaveBeenCalled()
  })

  it('draws the optimistic row inside the folder it was typed in', () => {
    render(<WorkspaceTree />)

    createFrom(rowFor('Reviewing'), 'feature/spike')

    expect(screen.getByText('feature/spike')).toBeInTheDocument()
  })
})

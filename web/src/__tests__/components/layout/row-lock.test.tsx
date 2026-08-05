/**
 * Locking and unlocking from the row context menu.
 *
 * The lock used to be the provider's alone: a protected branch was created
 * locked, re-locked on every poll, and there was no gesture anywhere in the app
 * that could disagree. These pin the user's half of it — that the menu offers
 * the verb that would change something, and that pressing it reaches the daemon
 * with the workspace's own scope.
 *
 * Automatic locking is untouched by any of this; the daemon-side precedence is
 * pinned in api/…/commands/set_lock_test.go.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, act } from '@testing-library/react'

const setWorkspaceLock = vi.hoisted(() => vi.fn(() => Promise.resolve()))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  setWorkspaceLock,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

const project: Project = { id: 'p1', name: 'harbour', path: '/p1', lastActivity: new Date(0) }

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'web',
  avatarLabel: 'W',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    // `main` — protected, so the daemon created it locked.
    { id: 'main', branch: 'main', status: 'locked', age: '', order: 0 },
    { id: 'a', branch: 'alpha', status: 'new', age: '', order: 1 },
  ],
  folders: [],
}

const rowEl = (id: string) => document.querySelector(`[data-ws-drop="${id}"]`) as HTMLElement
const menuItems = () =>
  Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent?.trim())
const clickItem = (label: string) => {
  const item = Array.from(document.querySelectorAll('[role="menuitem"]')).find(
    (el) => el.textContent?.trim() === label,
  )
  act(() => {
    fireEvent.click(item!)
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarSelectionStore.setState({ selected: new Set<string>() })
  useSidebarStore.setState({
    repos: [repo],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('the lock verbs on offer', () => {
  it('offers UNLOCK on a protected branch', () => {
    // The whole point of the override: main used to be locked with no way out.
    render(<WorkspaceTree />)

    fireEvent.contextMenu(rowEl('main'))

    expect(menuItems()).toContain('Unlock workspace')
    expect(menuItems()).not.toContain('Lock workspace')
  })

  it('offers LOCK on an ordinary branch', () => {
    render(<WorkspaceTree />)

    fireEvent.contextMenu(rowEl('a'))

    expect(menuItems()).toContain('Lock workspace')
    expect(menuItems()).not.toContain('Unlock workspace')
  })
})

describe('pressing them', () => {
  it('unlocks with the workspace’s own project and repo scope', () => {
    render(<WorkspaceTree />)
    fireEvent.contextMenu(rowEl('main'))

    clickItem('Unlock workspace')

    expect(setWorkspaceLock).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'main', false)
  })

  it('locks an ordinary branch', () => {
    render(<WorkspaceTree />)
    fireEvent.contextMenu(rowEl('a'))

    clickItem('Lock workspace')

    expect(setWorkspaceLock).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'a', true)
  })

  it('writes nothing to the sidebar itself', () => {
    // No optimistic write: the updated workspace arrives on its repo's WS
    // stream, which is also what re-renders the row's glyph. A lock the
    // frontend remembered would be a lock the daemon did not enforce.
    render(<WorkspaceTree />)
    fireEvent.contextMenu(rowEl('main'))

    clickItem('Unlock workspace')

    expect(useSidebarStore.getState().repos[0].workspaces[0].status).toBe('locked')
  })
})

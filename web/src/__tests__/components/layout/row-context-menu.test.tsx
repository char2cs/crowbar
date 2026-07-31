/**
 * The right-click menu, on the real tree.
 *
 * The rule worth pinning is whose rows it acts on: `dragSubjectsFor()` decides
 * that for a drag, and the menu reads the same function, so right-clicking
 * inside the multiselection takes the whole selection and right-clicking
 * outside it takes that row alone — without disturbing the selection.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  deleteWorkspace: vi.fn(() => Promise.resolve()),
  deleteRepo: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() => Promise.resolve()),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() => Promise.resolve({ id: 'new-folder' })),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

import { createFolder, placeWorkspace } from '@/lib/api/sidebar-placement'
import { deleteWorkspace } from '@/lib/api'
import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialSelectionState, useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
    { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
    { id: 'c', branch: 'gamma', status: 'new', age: '', order: 2 },
  ],
})

const rowEl = (id: string) =>
  document.querySelector<HTMLElement>(`[data-ws-drop="${id}"], [data-project-drop="${id}"]`)!

const menuItems = () =>
  Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)

const held = () => useRemovalTrayStore.getState().entries.map((e) => e.id)

beforeEach(() => {
  vi.clearAllMocks()
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarSelectionStore.setState(getInitialSelectionState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('what the menu acts on', () => {
  it('takes the multiselection when the clicked row is part of it', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('a'), { metaKey: true })
    fireEvent.click(rowEl('b'), { metaKey: true })

    fireEvent.contextMenu(rowEl('b'))

    expect(menuItems()).toEqual(['Group 2 into a folder', 'Remove 2 workspaces'])
  })

  // Pressing on an unselected row is not a way to extend a selection, and it
  // must not quietly act on rows the user is no longer pointing at.
  it('takes the clicked row alone when it is outside the selection', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('a'), { metaKey: true })

    fireEvent.contextMenu(rowEl('c'))

    expect(menuItems()).toEqual(['Group into a folder', 'Remove workspace'])
    expect([...useSidebarSelectionStore.getState().selected]).toEqual(['a'])
  })

  it('opens no menu on a project row', () => {
    render(<WorkspaceTree />)

    fireEvent.contextMenu(rowEl('p1'))

    expect(menuItems()).toEqual([])
  })
})

describe('the actions', () => {
  it('sends Remove through the tray, not through a delete', () => {
    render(<WorkspaceTree />)
    fireEvent.contextMenu(rowEl('a'))

    fireEvent.click(screen.getByText('Remove workspace'))

    expect(deleteWorkspace).not.toHaveBeenCalled()
    expect(held()).toEqual(['a'])
  })

  // Nothing in this file answers a modal prompt, and nothing needs to: the
  // gesture is "these belong together", so the folder is made at once under a
  // default name. A prompt in the way would leave `createFolder` uncalled here.
  it('creates the folder under a default name, then files the rows into it in order', async () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('a'), { metaKey: true })
    fireEvent.click(rowEl('b'), { metaKey: true })
    fireEvent.contextMenu(rowEl('b'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group 2 into a folder'))
    })

    expect(createFolder).toHaveBeenCalledWith('p1', 'r1', 'New folder', '')
    expect(placeWorkspace).toHaveBeenNthCalledWith(1, 'p1', 'r1', 'a', {
      folderId: 'new-folder',
      order: 0,
    })
    expect(placeWorkspace).toHaveBeenNthCalledWith(2, 'p1', 'r1', 'b', {
      folderId: 'new-folder',
      order: 1,
    })
  })

  // The selection has been spent by the grouping. Left lit, the collected rows
  // would wear the open workspace's treatment at the moment they move under a
  // folder that is itself asking to be named.
  it('spends the selection rather than leaving it lit', async () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('a'), { metaKey: true })
    fireEvent.click(rowEl('b'), { metaKey: true })
    fireEvent.contextMenu(rowEl('b'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group 2 into a folder'))
    })

    expect([...useSidebarSelectionStore.getState().selected]).toEqual([])
  })

  // The default name is a starting point, not the answer: the row arrives with
  // its editor already open and the name selected, so typing replaces it.
  it('leaves the new folder in the rename editor, with the default name selected', async () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('a'), { metaKey: true })
    fireEvent.click(rowEl('b'), { metaKey: true })
    fireEvent.contextMenu(rowEl('b'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group 2 into a folder'))
    })
    // The FolderDTO reaches the sidebar on its stream, a beat after the create
    // answered with the id.
    act(() => {
      useSidebarStore.setState({
        repos: [
          {
            ...repo(),
            folders: [{ id: 'new-folder', repoId: 'r1', name: 'New folder', order: 0 }],
          },
        ],
      })
    })

    const input = screen.getByDisplayValue<HTMLInputElement>('New folder')
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('New folder'.length)
  })
})

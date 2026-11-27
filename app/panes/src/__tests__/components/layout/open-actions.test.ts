import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, toastError, getHomeWorkspaceId } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  toastError: vi.fn(),
  getHomeWorkspaceId: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { resolveRow, handleOpen } from '@/components/layout/open-actions'
import type { Chat, Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { resetEntities, seedHomeTree } from '@/__tests__/__fixtures__/sidebar-entities'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  defaultBranch: 'main',
  workspaces: [],
  folders: [],
  ...over,
})

afterEach(() => {
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  resetEntities()
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

describe('resolveRow', () => {
  it('resolves a real workspace row', () => {
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    const found = resolveRow(repos, 'ws-a')
    expect(found?.repo.id).toBe('r1')
    expect(found?.subject).toEqual({
      kind: 'workspace',
      id: 'ws-a',
      repoId: 'r1',
      locked: false,
      parentId: undefined,
    })
  })

  it('resolves a folder row', () => {
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    const found = resolveRow(repos, 'f1')
    expect(found?.subject).toEqual({ kind: 'folder', id: 'f1', repoId: 'r1', parentId: undefined })
  })

  it('resolves the repo-home id as a workspace subject with no matching row', () => {
    const repos = [repo()]
    const found = resolveRow(repos, 'home-1')
    expect(found?.subject).toEqual({ kind: 'workspace', id: 'home-1', repoId: 'r1' })
  })

  it('returns null for an id in no repo', () => {
    expect(resolveRow([repo()], 'nope')).toBeNull()
  })
})

describe('handleOpen', () => {
  it('says the chat is still loading, and does not navigate, for a workspace row with no owning chat yet', () => {
    const navigate = vi.fn()
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    handleOpen('ws-a', repos, navigate)
    expect(navigate).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledWith("Can't open alpha yet — its chat is still loading")
  })

  it('toggles a folder instead of navigating', () => {
    const navigate = vi.fn()
    const toggle = vi.spyOn(treeUiStore.getState(), 'toggleChatRow')
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    expect(handleOpen('f1', repos, navigate)).toBe(false)
    expect(toggle).toHaveBeenCalledWith('f1')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('is a no-op for a row not in the given (removal-filtered) repos', () => {
    const navigate = vi.fn()
    expect(handleOpen('ghost', [repo()], navigate)).toBe(false)
    expect(navigate).not.toHaveBeenCalled()
  })

  describe('a chat row', () => {
    const withChat = (chat: Partial<Chat> & { id: string }) =>
      repo({
        workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
        chats: [{ repoId: 'r1', title: 'a chat', order: 0, ...chat }],
      })

    it('navigates to the workspace a WORKTREE chat owns, like a branch row', () => {
      const navigate = vi.fn()
      expect(handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)).toBe(true)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
    })

    it('navigates to the repo home when that is the workspace it owns', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'home-1' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'home-1' },
      })
    })

    it('opens into a pane immediately after navigating, even while the active PANE already holds a chat from a DIFFERENT workspace', async () => {
      resetWindowPaneStoreForTests()
      setActiveWorkspaceId('ws-other')
      windowPaneStore.getState().paneActions.openChat('already-open-chat')
      const navigate = vi.fn()

      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)
      await Promise.resolve()
      await Promise.resolve()

      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
      const panes = windowPaneStore.getState().panes
      expect(Object.values(panes).some((p) => p.chatId === 'c1')).toBe(true)
      resetWindowPaneStoreForTests()
    })

    it('opens a BUBBLE into its nearest ancestor workspace (the repo home, at the root)', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'home-1' },
      })
    })

    it('opens a BUBBLE nested under a real workspace into THAT workspace', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', parentId: 'ws-a' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
    })

    it('folds rather than routing to a workspace that is not in this repo', () => {
      const navigate = vi.fn()
      const toggle = vi.spyOn(treeUiStore.getState(), 'toggleChatRow')
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-in-another-repo' })], navigate)
      expect(toggle).toHaveBeenCalledWith('c1')
      expect(navigate).not.toHaveBeenCalled()
    })

    describe('opening it into a pane (the workspace is already on screen)', () => {
      beforeEach(() => {
        resetWindowPaneStoreForTests()
        setActiveWorkspaceId('ws-a')
      })
      afterEach(() => {
        resetWindowPaneStoreForTests()
      })

      it('fills the pane already on screen when it is empty, without navigating away', () => {
        const navigate = vi.fn()
        handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)

        expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
        expect(navigate).not.toHaveBeenCalled()
      })

      it('gives a second clicked chat a pane of its OWN, and merges nothing', () => {
        const navigate = vi.fn()
        const repos = [
          repo({
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            chats: [
              { id: 'c1', repoId: 'r1', title: 'one', order: 0, workspaceId: 'ws-a' },
              { id: 'c2', repoId: 'r1', title: 'two', order: 1, workspaceId: 'ws-a' },
            ],
          }),
        ]

        handleOpen('c1', repos, navigate)
        handleOpen('c2', repos, navigate)

        const panes = windowPaneStore.getState().panes
        expect(panes[ROOT_PANE_ID]?.chatId).toBe('c1')
        expect(Object.values(panes).find((p) => p.chatId === 'c2')?.id).not.toBe(ROOT_PANE_ID)
        expect(windowPaneStore.getState().viewOrder).toHaveLength(2)
      })

      it('a chat already up is gone TO rather than opened a second time', () => {
        const navigate = vi.fn()
        const repos = [
          repo({
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            chats: [
              { id: 'c1', repoId: 'r1', title: 'one', order: 0, workspaceId: 'ws-a' },
              { id: 'c2', repoId: 'r1', title: 'two', order: 1, workspaceId: 'ws-a' },
            ],
          }),
        ]
        handleOpen('c1', repos, navigate)
        handleOpen('c2', repos, navigate)
        const paneCount = Object.keys(windowPaneStore.getState().panes).length

        handleOpen('c1', repos, navigate)

        expect(Object.keys(windowPaneStore.getState().panes)).toHaveLength(paneCount)
        expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
      })
    })
  })

  describe('a project-home row', () => {
    beforeEach(() => {
      resetWindowPaneStoreForTests()
    })
    afterEach(() => {
      resetWindowPaneStoreForTests()
    })

    it('opens an existing home chat in place when home is already active', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('p1', {
        chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
        folders: [],
      })
      setActiveWorkspaceId('home-ws-1')
      const navigate = vi.fn()

      handleOpen('c1', [], navigate)

      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
      expect(navigate).not.toHaveBeenCalled()
    })

    it('navigates to project home first when it is not already active, then opens', async () => {
      setActiveWorkspaceId('unrelated-ws')
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('p1', {
        chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
        folders: [],
      })
      const navigate = vi.fn(async () => {
        setActiveWorkspaceId('home-ws-1')
      })

      handleOpen('c1', [], navigate)

      await vi.waitFor(() => {
        expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
      })
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/home',
        params: { projectId: 'p1' },
      })
    })

    it('toggles fold for a home folder instead of opening it', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('p1', {
        chats: [],
        folders: [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }],
      })
      const toggle = vi.spyOn(treeUiStore.getState(), 'toggleChatRow')
      const navigate = vi.fn()

      handleOpen('f1', [], navigate)

      expect(toggle).toHaveBeenCalledWith('f1')
      expect(navigate).not.toHaveBeenCalled()
    })

    it('resolves the right project among several visible home trees', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'p2' ? 'home-ws-2' : 'home-ws-1',
      )
      seedHomeTree('p1', {
        chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: '', order: 0 }],
        folders: [],
      })
      seedHomeTree('p2', {
        chats: [{ id: 'c2', repoId: '', workspaceId: 'home-ws-2', title: '', order: 0 }],
        folders: [],
      })
      setActiveWorkspaceId('home-ws-2')
      const navigate = vi.fn()

      handleOpen('c2', [], navigate)

      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c2')
      expect(navigate).not.toHaveBeenCalled()
    })
  })
})

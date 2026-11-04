/** A branch row is addressed by its owning chat, and every verb treats it as its workspace. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, deleteChat } = vi.hoisted(() => ({
  createChat: vi.fn((id: string) => Promise.resolve(id)),
  createChatWithOwnWorktree: vi.fn((id: string) => Promise.resolve({ id, workspaceId: 'ws-new' })),
  deleteChat: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/agent/api/agent-chat-commands', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-commands')>()),
  deleteChat,
}))

import { resolveChatRow, resolveRow } from '@/components/layout/open-actions'
import { handleCreate, confirmCreateDraft } from '@/components/layout/create-actions'
import { createThread } from '@/__tests__/__fixtures__/draft-thread'
import { handleTrash } from '@/components/layout/trash-actions'
import type { Chat, Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  useAgentProvidersStore.setState({ status: 'ready', providers: [], remote: {} })
})

const ANY = expect.any(String)

function confirmArmedBranchName(name = 'typed-branch'): void {
  if (!treeUiStore.getState().draft) throw new Error('no draft is open')
  confirmCreateDraft(name, vi.fn())
}

/**
 * A `branch` row's id is the id of the CHAT that owns its workspace
 * (`deriveRows`), which puts it in the chat id space while making it no
 * chat at all. Every dispatcher here picks its behaviour by which space an id
 * falls in, so each one has to be able to tell the two apart — the bug this
 * closes is a locked branch's "+" going silently inert because `resolveChatRow`
 * matched its row and returned early.
 */
describe('a branch row is addressed by its owning chat, and is still a workspace', () => {
  const branchRow = (id: string, workspaceId: string): Chat => ({
    id,
    repoId: 'r1',
    ownsWorktree: true,
    workspaceId,
    title: '',
    order: 0,
  })

  const lockedRepo = () =>
    repo({
      workspaces: [
        {
          id: 'ws-locked',
          branch: 'develop',
          age: '',
          status: 'locked',
          owningChatId: 'develop-row',
        },
        { id: 'ws-open', branch: 'feature/x', age: '' },
      ],
      chats: [branchRow('home-row', 'home-1'), branchRow('develop-row', 'ws-locked')],
    })

  it('is not a chat row', () => {
    expect(resolveChatRow([lockedRepo()], 'develop-row')).toBeNull()
  })

  it('resolves to the WORKSPACE it draws, so drag and removal see one id space', () => {
    const found = resolveRow([lockedRepo()], 'develop-row')
    expect(found?.subject).toMatchObject({ kind: 'workspace', id: 'ws-locked', locked: true })
  })

  it('the repo-home row resolves to the default workspace', () => {
    expect(resolveRow([lockedRepo()], 'home-row')?.subject).toMatchObject({
      kind: 'workspace',
      id: 'home-1',
    })
  })

  it('its "+" creates a workspace under the OWNING CHAT id — the id the daemon places by', () => {
    seedRepos([lockedRepo()])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('develop-row', 'workspace', vi.fn())
    confirmArmedBranchName()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'claude',
      'develop-row',
      'typed-branch',
    )
  })

  it('its thread "+" runs in the WORKSPACE, not in the row id', async () => {
    seedRepos([lockedRepo()])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await createThread('develop-row', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'ws-locked',
      'claude',
      'develop-row',
      undefined,
    )
  })

  it('its trash takes the WORKSPACE path — refused as locked, never deleteChat', () => {
    seedRepos([lockedRepo()])

    // A branch row can only ever be a locked branch or a repo home, and
    // `planRemoval`'s `draftFor` refuses both — so the tray staying empty is
    // the REFUSAL, and on its own it is indistinguishable from doing nothing.
    // The ordinary workspace below is what tells those two apart: the same
    // call, in the same repo, does reach the tray.
    expect(handleTrash('develop-row')).toBe(false)
    expect(deleteChat).not.toHaveBeenCalled()
    expect(treeUiStore.getState().removals).toEqual([])

    expect(handleTrash('ws-open')).toBe(true)
    expect(treeUiStore.getState().removals).toHaveLength(1)
    expect(deleteChat).not.toHaveBeenCalled()
  })
})

// A provider list belongs to a daemon: forking a remote workspace starts its first
// chat with a provider THAT host offers, never one only the local daemon has.
describe('forking on an install that has paired a remote host', () => {
  it('starts the fork with a provider its own host offers', async () => {
    resetHostWorld()
    pairRemoteHost('host-1')
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    seedRepos([
      repo({
        id: 'r-remote',
        projectId: 'p-remote',
        workspaces: [{ id: 'ws-remote', branch: 'main', age: '', order: 0 }],
      }),
    ])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
      remote: { 'host-1': [{ id: 'codex-remote', enabled: true }] as never },
    })

    await handleCreate('ws-remote', 'workspace', vi.fn())
    confirmArmedBranchName()

    expect(createChatWithOwnWorktree.mock.calls[0]?.slice(2, 5)).toEqual([
      'p-remote',
      'r-remote',
      'codex-remote',
    ])
    resetHostWorld()
  })
})

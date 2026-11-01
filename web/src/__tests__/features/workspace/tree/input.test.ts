import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { deriveRows } from '@/features/workspace/tree/rows'
import { treeInputFor, useTreeInput } from '@/features/workspace/tree/input'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { treeUiStore, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import type { Operation, OperationItem } from '@/lib/store/operations'
import type { Chat, Folder } from '@/lib/store/sidebar'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { HOME_ROW_ID, homeRows } from '@/__tests__/__fixtures__/tree-rows'
import {
  __resetHomeWorkspaceResolverForTest,
  ensureHomeWorkspaceResolved,
} from '@/features/workspace/lib/home-workspace-resolver'

const fetchHomeWorkspace = vi.fn()
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspace(projectId),
}))

const NOT_A_DIR = {
  code: 'internal' as const,
  message: 'could not create work tree: Not a directory',
}

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-home',
  defaultOwningChatId: 'home-row',
  defaultBranch: 'main',
  workspaces: [],
  folders: [],
  chats: [],
  ...over,
})

const item = (id: string, label: string, over: Partial<OperationItem> = {}): OperationItem => ({
  ref: { kind: 'chat', id },
  label,
  state: 'running',
  ...over,
})

const importOp = (over: Partial<Operation> = {}): Operation => ({
  id: 'op-1',
  kind: 'branch.import',
  subject: { kind: 'repo', id: 'r1' },
  projectId: 'p1',
  label: 'Import 2 branches',
  state: 'running',
  items: [item('chat-a', 'bad/x'), item('chat-b', 'good')],
  dismissed: false,
  version: 2,
  createdAt: '2026-10-29T10:00:00Z',
  updatedAt: '2026-10-29T10:00:01Z',
  ...over,
})

beforeEach(() => {
  useSidebarStore.setState({ ...getInitialState(), repos: [repo()] })
  useHomeTreeStore.setState({ trees: {} })
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  useFolderSignalStore.setState({ seededRepoIds: new Set(), seededWorkspaceRepoIds: new Set() })
  resetOperations()
  __resetHomeWorkspaceResolverForTest()
  fetchHomeWorkspace.mockReset()
})

const ids = (projectId: string) => deriveRows(treeInputFor(projectId)).map((r) => r.id)

describe('one project at a time', () => {
  it("draws only the given project's repos, every repo it holds", () => {
    useSidebarStore.setState({
      repos: [
        repo({ workspaces: [{ id: 'ws-1', branch: 'alpha', age: '' }] }),
        repo({ id: 'r2', defaultWorkspaceId: 'home-2', defaultOwningChatId: 'home-2-row' }),
        repo({ id: 'r3', projectId: 'p2', defaultOwningChatId: 'home-3-row' }),
      ],
    })
    expect(ids('p1')).toEqual(['home-row', 'ws-1', 'home-2-row'])
    expect(ids('p2')).toEqual(['home-3-row'])
    expect(ids('unknown')).toEqual([])
  })

  it('a repo with no project yet belongs to no space', () => {
    useSidebarStore.setState({ repos: [repo({ projectId: undefined })] })
    expect(ids('p1')).toEqual([])
  })
})

describe('useTreeInput', () => {
  const homeChat = { id: 'c-1', repoId: '', workspaceId: 'home-ws', title: 'Plan', order: 0 }
  const drawn = (derive: (projectId: string) => ReturnType<typeof treeInputFor>) =>
    deriveRows(derive('p1')).map((r) => r.id)

  // A row id must never change under the user, so a repo waits for its tree or workspaces seed.
  it('draws a repo once its tree or its workspaces have seeded', () => {
    const { result } = renderHook(() => useTreeInput())
    expect(drawn(result.current)).toEqual([])
    act(() => useFolderSignalStore.setState({ seededWorkspaceRepoIds: new Set(['r1']) }))
    expect(drawn(result.current)).toEqual(['home-row'])
  })

  it('re-derives the home rows when the home workspace resolves after mount', async () => {
    useSidebarStore.setState({ repos: [] })
    useHomeTreeStore.setState({ trees: { p1: { chats: [homeChat], folders: [] } } })
    fetchHomeWorkspace.mockResolvedValue({ id: 'home-ws', owningChatId: 'home-owner' })
    // Derived during render, as the surface does: only a re-render can show the resolved home.
    const { result } = renderHook(() => drawn(useTreeInput()))
    expect(result.current).toEqual([])
    act(() => ensureHomeWorkspaceResolved('p1'))
    await waitFor(() => expect(result.current).toEqual(['c-1']))
  })

  it("hides a held home row's descendants and keeps the held row, marked, on screen", async () => {
    useSidebarStore.setState({ repos: [] })
    const thread = { ...homeChat, id: 'c-2', parentId: 'c-1' }
    useHomeTreeStore.setState({ trees: { p1: { chats: [homeChat, thread], folders: [] } } })
    fetchHomeWorkspace.mockResolvedValue({ id: 'home-ws', owningChatId: 'home-owner' })
    ensureHomeWorkspaceResolved('p1')
    await waitFor(() => expect(ids('p1')).toEqual(['c-1', 'c-2']))
    const held = { kind: 'chat', id: 'c-1', entryId: 'e1', deadlineAt: 1, extra: 1 }
    treeUiStore.setState({
      removals: [{ ...held, hiddenIds: ['c-1', 'c-2'] } as unknown as RemovalEntry],
      hiddenIds: new Set(['c-1', 'c-2']),
    })
    const rows = deriveRows(treeInputFor('p1'))
    expect(rows.map((r) => [r.id, r.removal?.entryId])).toEqual([['c-1', 'e1']])
  })
})

/** The create rows drawn for `ops`, beside the repos the sidebar store holds now. */
const rows = (ops: Operation[]) => {
  for (const op of ops) streamOperation(op)
  return deriveRows(treeInputFor('p1')).filter((r) => r.operation)
}

describe('operation rows', () => {
  it('a running import draws one creating row per branch under its repo', () => {
    expect(rows([importOp()])).toMatchObject([
      { id: 'chat-a', kind: 'branch', label: 'bad/x', parentId: 'home-row', working: true },
      { id: 'chat-b', kind: 'branch', label: 'good', parentId: 'home-row', working: true },
    ])
    expect(rows([importOp()])[0]?.operation).toEqual({
      opId: 'op-1',
      state: 'running',
      problem: null,
    })
  })

  // A repo add's subject is the project; its failed branch is drawn under the
  // repo it added, and the repo itself is no row.
  it('a partly failed repo add draws its failed branch under the added repo', () => {
    const added = importOp({
      kind: 'repo.create',
      subject: { kind: 'project', id: 'p1' },
      label: 'Add crowbar',
      state: 'failed',
      error: { code: 'partial', message: '1 of 3 failed' },
      items: [
        { ref: { kind: 'repo', id: 'r1' }, label: 'Add crowbar', state: 'succeeded' },
        { ref: { kind: 'workspace', id: 'ws-home' }, label: 'main', state: 'succeeded' },
        {
          ref: { kind: 'workspace', id: 'ws-dev' },
          label: 'develop',
          state: 'failed',
          error: NOT_A_DIR,
        },
      ],
    })
    const drawn = rows([added])
    expect(drawn).toMatchObject([
      { id: 'ws-dev', kind: 'branch', label: 'develop', parentId: 'home-row' },
    ])
    expect(drawn[0]?.operation).toEqual({ opId: 'op-1', state: 'failed', problem: NOT_A_DIR })
  })

  // The user's bug: an import that failed said nothing, so the reason must be on screen.
  it('a failed item draws its reason and a Dismiss action', () => {
    const failed = importOp({
      state: 'failed',
      error: { code: 'partial', message: '1 of 2 failed' },
      items: [
        item('chat-a', 'bad/x', { state: 'failed', error: NOT_A_DIR }),
        item('chat-b', 'good', { state: 'succeeded' }),
      ],
    })
    expect(rows([failed])).toMatchObject([
      { id: 'chat-a', label: 'bad/x', working: false },
      { id: 'chat-b', working: true },
    ])
    expect(rows([failed])[0]?.operation).toEqual({
      opId: 'op-1',
      state: 'failed',
      problem: NOT_A_DIR,
    })
  })

  it('a failed item whose entity exists draws only the entity', () => {
    useSidebarStore.setState({
      repos: [repo({ chats: [{ id: 'chat-a', repoId: 'r1', title: '', order: 0 }] })],
    })
    const failed = importOp({
      state: 'failed',
      items: [item('chat-a', 'bad/x', { state: 'failed', error: NOT_A_DIR })],
    })
    expect(rows([failed])).toEqual([])
  })

  it('an interrupted import row says Crowbar stopped before it finished', () => {
    const interrupted = importOp({
      state: 'failed',
      error: { code: 'interrupted', message: 'Crowbar stopped before this finished' },
    })
    expect(rows([interrupted]).map((r) => r.operation?.problem?.message)).toEqual([
      'Crowbar stopped before this finished',
      'Crowbar stopped before this finished',
    ])
  })

  it('a succeeded item disappears once its entity is present', () => {
    const done = importOp({
      state: 'succeeded',
      items: [item('chat-b', 'good', { state: 'succeeded' })],
    })
    expect(rows([done])).toMatchObject([{ id: 'chat-b', working: true }])
    useSidebarStore.setState({
      repos: [repo({ chats: [{ id: 'chat-b', repoId: 'r1', title: '', order: 0 }] })],
    })
    expect(rows([done])).toEqual([])
  })

  // A ghost after reload: the snapshot replays a create whose chat was deleted since.
  it('a succeeded item replayed on connect draws nothing: its entity was deleted, not in flight', () => {
    const replayed = importOp({
      state: 'succeeded',
      snapshot: true,
      items: [item('chat-b', 'good', { state: 'succeeded' })],
    })
    expect(rows([replayed])).toEqual([])
    const partial = importOp({
      state: 'failed',
      snapshot: true,
      items: [
        item('chat-a', 'bad/x', { state: 'failed', error: NOT_A_DIR }),
        item('chat-b', 'good', { state: 'succeeded' }),
      ],
    })
    expect(rows([partial]).map((r) => r.id)).toEqual(['chat-a'])
  })

  it('a dismissed failed operation draws nothing', () => {
    const dismissed = importOp({ state: 'failed', dismissed: true, error: NOT_A_DIR })
    expect(rows([dismissed])).toEqual([])
  })

  // A git or workspace verb names the workspace; the row is drawn by its owning chat.
  it('a failed push on the workspace shows on its chat row', () => {
    const placeholder = repo({
      workspaces: [
        {
          id: 'ws-1',
          branch: 'feature',
          age: '',
          order: 0,
          owningChatId: 'chat-1',
          provisioning: 'placeholder',
        },
      ],
      chats: [
        {
          id: 'chat-1',
          repoId: 'r1',
          workspaceId: 'ws-1',
          ownsWorktree: true,
          title: 't',
          order: 0,
        },
      ],
    })
    streamOperation(
      importOp({
        id: 'push',
        kind: 'workspace.retry_provision',
        subject: { kind: 'workspace', id: 'ws-1' },
        state: 'failed',
        items: [],
        error: { code: 'conflict', message: 'branch is still held' },
      }),
    )
    useSidebarStore.setState({ repos: [placeholder] })
    const row = deriveRows(treeInputFor('p1')).find((r) => r.id === 'chat-1')
    expect(row?.placeholderReason).toContain('branch is still held')
    // Not a create: no row of its own.
    expect(rows([])).toEqual([])
  })
})

const HOME_WS_ID = 'home-ws-1'

function homeFolder(over: Partial<Folder> & { id: string; name: string }): Folder {
  return { repoId: '', order: 0, ...over }
}

function homeChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: '', order: 0, ...over }
}

function homeOwningChat(): Chat {
  return homeChat({ id: HOME_ROW_ID, title: '', workspaceId: HOME_WS_ID, ownsWorktree: true })
}

describe('a project home', () => {
  // Explicit user correction: a first version drew a container "Home" row for every home
  // chat/folder to nest under, mirroring a repo's own home row — rejected outright.
  it('draws no row for the owning branch chat itself', () => {
    const rows = homeRows(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
    expect(rows.find((r) => r.id === HOME_ROW_ID)).toBeUndefined()
  })

  // What `/v0/projects/:p/home/chats` really sends: the owner row carries `workspaceId` but NO
  // worktree marker (`chatDTOFromWire` can only set `ownsWorktree` off `worktree.owningChatId`, and
  // the home list is built with a nil worktree closure), so the owner is only knowable from the
  it('the owner is not drawn when its wire row has no ownsWorktree marker but the resolver named it', () => {
    const wireOwner = homeChat({ id: HOME_ROW_ID, title: '', workspaceId: HOME_WS_ID })
    const thread = homeChat({ id: 'c-1', title: 'chat-one', workspaceId: HOME_WS_ID })
    const rows = homeRows(HOME_WS_ID, [wireOwner, thread], [], HOME_ROW_ID)
    expect(rows.map((r) => r.id)).toEqual(['c-1'])
  })

  // REGRESSION: a legacy home whose GET /home elected the user's earliest conversation as its owner
  // hid that conversation from the tree. The wire row's own marker is authoritative: a named chat
  // that says it does NOT own the worktree is drawn, and only a marked owner (or none) is kept off.
  it('draws a chat the resolver named but the list marks as not owning the worktree', () => {
    const conversation = homeChat({
      id: 'c-plan',
      title: 'plan the release',
      workspaceId: HOME_WS_ID,
      ownsWorktree: false,
    })
    const rows = homeRows(HOME_WS_ID, [conversation, homeOwningChat()], [], 'c-plan')
    expect(rows.map((r) => r.id)).toEqual(['c-plan'])
  })

  // Task 9: the owning chat is minted chat-first, atomically, at creation — there is no boot
  // backfill left to race, so a caller catching this window (its own creation landed, its
  // chat/folder tree's first seed has not) degrades gracefully instead of throwing: no rows, not a
  it('degrades to no rows, never throws, while the owning chat has not seeded yet', () => {
    expect(() => homeRows(HOME_WS_ID, [])).not.toThrow()
    expect(homeRows(HOME_WS_ID, [])).toEqual([])
  })

  it('a thread on the home workspace becomes a top-level chat-kind row, parentId null', () => {
    const thread = homeChat({ id: 'c-1', title: 'Fix the thing', workspaceId: HOME_WS_ID })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat(), thread])
    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.parentId).toBeNull()
    expect(row?.ownsWorktree).toBe(false)
  })

  it('an untitled thread falls back to the shared placeholder label', () => {
    const thread = homeChat({ id: 'c-1', title: '', workspaceId: HOME_WS_ID })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat(), thread])
    expect(rows.find((r) => r.id === 'c-1')?.label).toBe(UNTITLED_CHAT_LABEL)
  })

  it('a folder becomes a top-level folder-kind row, parentId null', () => {
    const folder = homeFolder({ id: 'f-1', name: 'Notes' })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat()], [folder])
    const row = rows.find((r) => r.id === 'f-1')
    expect(row?.kind).toBe('folder')
    expect(row?.parentId).toBeNull()
  })

  // Project home has no repo, so no folder inside it ever owns a worktree —
  // unlike deriveRows's own folder, which always does. Its own "+"
  // must never offer Fork: there is no worktree for it to clone.
  it('a home folder never owns a worktree, so its own "+" cannot fork', () => {
    const folder = homeFolder({ id: 'f-1', name: 'Notes' })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat()], [folder])
    expect(rows.find((r) => r.id === 'f-1')?.ownsWorktree).toBe(false)
  })

  it('a thread filed into a folder nests under the folder, not at the top level', () => {
    const folder = homeFolder({ id: 'f-1', name: 'Notes' })
    const thread = homeChat({
      id: 'c-1',
      title: 'Filed',
      workspaceId: HOME_WS_ID,
      parentId: 'f-1',
    })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat(), thread], [folder])
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe('f-1')
  })

  it('defaults to no chats/folders — just the (empty) owning-chat exclusion', () => {
    const rows = homeRows(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
  })

  // Task 3 put a repo's `order`/`folderId` on its own `Node` row, computed server-side against
  // these SAME real chat/folder siblings — so a chat's rendered `order` now has to carry its REAL
  // wire value through, not a 0..n-1 index compacted from whichever chats/folders happened to reach
  it("a chat's rendered order is its real wire value, not a index compacted from array position", () => {
    const chatA = homeChat({ id: 'c-a', title: 'a', workspaceId: HOME_WS_ID, order: 0 })
    const chatB = homeChat({ id: 'c-b', title: 'b', workspaceId: HOME_WS_ID, order: 5 })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat(), chatA, chatB])
    expect(rows.find((r) => r.id === 'c-a')?.order).toBe(0)
    expect(rows.find((r) => r.id === 'c-b')?.order).toBe(5)
  })

  // Task 9: `owningChatOf` files a NEW chat/folder directly under its parent workspace's own id —
  // no owning-chat lookup — the instant that parent's `Node{Kind:workspace}` row exists, which
  // project home always has.
  it('a chat parented onto the raw home-workspace id still renders top-level', () => {
    const child = homeChat({ id: 'c-1', title: 'New-style child', parentId: HOME_WS_ID })
    const rows = homeRows(HOME_WS_ID, [homeOwningChat(), child])
    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.parentId).toBeNull()
  })
})

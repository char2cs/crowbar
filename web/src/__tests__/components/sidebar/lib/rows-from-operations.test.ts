import { beforeEach, describe, expect, it } from 'vitest'
import {
  placeUnderRepoHomes,
  rowsFromOperations,
} from '@/components/sidebar/lib/rows-from-operations'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import {
  newestProblem,
  selectFailedOps,
  operationsState,
  type Operation,
  type OperationItem,
} from '@/lib/store/operations'

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

const rows = (ops: Operation[]) =>
  placeUnderRepoHomes(rowsFromOperations(ops, { projectId: 'p1' }), [repo()])

beforeEach(() => {
  useSidebarStore.setState({ ...getInitialState(), repos: [repo()] })
  resetOperations()
})

describe('rowsFromOperations', () => {
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
    const present = new Set(['r1', 'ws-home'])
    const drawn = placeUnderRepoHomes(
      rowsFromOperations([added], { projectId: 'p1', repoId: 'r1' }, (ref) => present.has(ref.id)),
      [repo()],
    )
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
    const failed = selectFailedOps(operationsState())
    const row = rowsFromRepo(placeholder, (...ids) => newestProblem(failed, ids)).find(
      (r) => r.id === 'chat-1',
    )
    expect(row?.placeholderReason).toContain('branch is still held')
    // Not a create: no row of its own.
    expect(rows(Object.values(operationsState().ops))).toEqual([])
  })
})

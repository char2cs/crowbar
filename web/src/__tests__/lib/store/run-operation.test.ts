import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { applyOperationFrame, useOperationsStore, type Operation } from '@/lib/store/operations'
import {
  dismissOperation,
  runOperation,
  whenSettled,
  type OperationReply,
} from '@/lib/store/run-operation'

const push = {
  kind: 'git.push',
  subject: { kind: 'workspace' as const, id: 'ws1' },
  projectId: 'p1',
  label: 'Push',
}

function deferred() {
  let resolve!: (reply: OperationReply) => void
  let reject!: (err: unknown) => void
  const promise = new Promise<OperationReply>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function server(id: string, over: Partial<Operation>): Operation {
  return {
    ...push,
    id,
    state: 'accepted',
    items: [],
    dismissed: false,
    version: 1,
    createdAt: '2026-10-28T12:00:00Z',
    updatedAt: '2026-10-28T12:00:00Z',
    ...over,
  }
}

function repoWith(over: Partial<Repo>): Repo {
  return { id: 'r1', name: 'r', avatarLabel: 'R', avatarColor: '#000', workspaces: [], ...over }
}

const ops = () => useOperationsStore.getState().ops

beforeEach(() => {
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useSidebarStore.setState(getInitialState())
})

describe('runOperation', () => {
  it('inserts a local accepted operation before send resolves', () => {
    const reply = deferred()
    let sentAs = ''
    const op = runOperation(push, (id) => {
      sentAs = id
      return reply.promise
    })

    expect(sentAs).toBe(op.id)
    expect(ops()[op.id]).toMatchObject({ state: 'accepted', local: true, label: 'Push' })
  })

  it('a 202 reply adopts the server operation', async () => {
    const op = runOperation(push, async (id) => ({
      operation: server(id, { label: 'Push main' }),
    }))

    await whenSettledOrState(op.id, (o) => !o.local)
    expect(ops()[op.id]).toMatchObject({ label: 'Push main', version: 1 })
    expect(ops()[op.id].local).toBeUndefined()
  })

  it('a synchronous reply settles the local operation with its items', async () => {
    const item = {
      ref: { kind: 'chat' as const, id: 'c1' },
      label: 'a',
      state: 'succeeded' as const,
    }
    const op = runOperation(push, async () => ({ items: [item] }))

    await whenSettledOrState(op.id, (o) => o.state === 'succeeded')
    expect(ops()[op.id].items).toEqual([item])
  })

  it('a refused send fails the local operation with the problem', async () => {
    const op = runOperation(push, async () => {
      throw new ApiError('no remote', 409, 'conflict')
    })

    await expect(whenSettled(op.id)).rejects.toMatchObject({
      problem: { code: 'conflict', message: 'no remote' },
    })
    expect(ops()[op.id].state).toBe('failed')
  })
})

/** Resolves once the stored operation matches. */
function whenSettledOrState(id: string, match: (op: Operation) => boolean): Promise<void> {
  return new Promise((resolve) => {
    const check = () => {
      const op = useOperationsStore.getState().ops[id]
      if (op && match(op)) {
        stop()
        resolve()
      }
    }
    const stop = useOperationsStore.subscribe(check)
    check()
  })
}

describe('whenSettled', () => {
  it('resolves once the operation succeeded and every created item is present', async () => {
    const id = 'op-import'
    const chat = {
      ref: { kind: 'chat' as const, id: 'c1' },
      label: 'a',
      state: 'succeeded' as const,
    }
    applyOperationFrame(server(id, { kind: 'branch.import', state: 'succeeded', items: [chat] }))
    let settled = false
    const done = whenSettled(id).then(() => (settled = true))

    await Promise.resolve()
    expect(settled).toBe(false)
    useSidebarStore.setState({
      repos: [repoWith({ chats: [{ id: 'c1', repoId: 'r1', title: 'a', order: 0 }] })],
    })
    await done
    expect(settled).toBe(true)
  })

  it('whenSettled rejects with OperationFailedError carrying the problem', async () => {
    applyOperationFrame(server('op-f', { state: 'running' }))
    const settled = whenSettled('op-f')

    applyOperationFrame(
      server('op-f', { state: 'failed', version: 2, error: { code: 'internal', message: 'gone' } }),
    )

    const err = await settled.catch((e: unknown) => e)
    expect(err).toBeInstanceOf(OperationFailedError)
    expect(err).toMatchObject({ operationId: 'op-f', problem: { message: 'gone' } })
  })

  it('whenSettled resolves a delete once its items are absent', async () => {
    useSidebarStore.setState({ repos: [repoWith({})] })
    const repo = {
      ref: { kind: 'repo' as const, id: 'r1' },
      label: 'r',
      state: 'succeeded' as const,
    }
    applyOperationFrame(server('op-d', { kind: 'repo.delete', state: 'succeeded', items: [repo] }))
    let settled = false
    const done = whenSettled('op-d').then(() => (settled = true))

    await Promise.resolve()
    expect(settled).toBe(false)
    useSidebarStore.setState({ repos: [] })
    await done
  })

  it('whenSettled for a branch create waits for the chat and its workspace', async () => {
    const items = [
      { ref: { kind: 'chat' as const, id: 'c1' }, label: 'c', state: 'succeeded' as const },
      { ref: { kind: 'workspace' as const, id: 'ws9' }, label: 'w', state: 'succeeded' as const },
    ]
    applyOperationFrame(server('op-b', { kind: 'branch.create', state: 'succeeded', items }))
    let settled = false
    const done = whenSettled('op-b').then(() => (settled = true))

    useSidebarStore.setState({
      repos: [repoWith({ chats: [{ id: 'c1', repoId: 'r1', title: 'c', order: 0 }] })],
    })
    await Promise.resolve()
    expect(settled).toBe(false)
    useSidebarStore.setState({
      repos: [
        repoWith({
          chats: [{ id: 'c1', repoId: 'r1', title: 'c', order: 0 }],
          workspaces: [{ id: 'ws9', branch: 'b', age: '' }],
        }),
      ],
    })
    await done
    expect(settled).toBe(true)
  })

  it('whenSettled for a git push resolves on succeeded alone', async () => {
    applyOperationFrame(server('op-p', { state: 'running' }))
    const done = whenSettled('op-p')

    applyOperationFrame(server('op-p', { state: 'succeeded', version: 2 }))

    await expect(done).resolves.toMatchObject({ state: 'succeeded' })
  })
})

afterEach(() => vi.unstubAllGlobals())

describe('dismissOperation', () => {
  it('asks the daemon to dismiss and keeps the dismissed row it answers', async () => {
    const failed = server('op-x', { state: 'failed', version: 3 })
    applyOperationFrame(failed)
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ success: true, data: { ...failed, dismissed: true, version: 4 } }),
        ),
      )
    vi.stubGlobal('fetch', fetchMock)

    await dismissOperation('op-x')

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/v0\/operations\/op-x$/),
      expect.objectContaining({ method: 'DELETE' }),
    )
    expect(ops()['op-x']).toMatchObject({ dismissed: true, version: 4 })
  })
})

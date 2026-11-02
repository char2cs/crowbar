import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState, subscribeOperations, type Operation } from '@/core/entities/operations'
import {
  dismissOperation,
  runOperation,
  whenSettled,
  type OperationReply,
} from '@/core/entities/run-operation'

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

const ops = () => operationsState().ops

beforeEach(() => {
  resetOperations()
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
      const op = operationsState().ops[id]
      if (op && match(op)) {
        stop()
        resolve()
      }
    }
    const stop = subscribeOperations(check)
    check()
  })
}

describe('whenSettled', () => {
  it('whenSettled rejects with OperationFailedError carrying the problem', async () => {
    streamOperation(server('op-f', { state: 'running' }))
    const settled = whenSettled('op-f')

    streamOperation(
      server('op-f', { state: 'failed', version: 2, error: { code: 'internal', message: 'gone' } }),
    )

    const err = await settled.catch((e: unknown) => e)
    expect(err).toBeInstanceOf(OperationFailedError)
    expect(err).toMatchObject({ operationId: 'op-f', problem: { message: 'gone' } })
  })

  // The daemon publishes an operation's entities before its succeeded frame on one scope.
  it('whenSettled for a create resolves on succeeded, its entities already sent', async () => {
    const items = [
      { ref: { kind: 'chat' as const, id: 'c1' }, label: 'c', state: 'succeeded' as const },
    ]
    streamOperation(server('op-b', { kind: 'branch.create', state: 'running', items }))
    const done = whenSettled('op-b')

    streamOperation(
      server('op-b', { kind: 'branch.create', state: 'succeeded', version: 2, items }),
    )

    await expect(done).resolves.toMatchObject({ state: 'succeeded' })
  })

  it('whenSettled for a git push resolves on succeeded alone', async () => {
    streamOperation(server('op-p', { state: 'running' }))
    const done = whenSettled('op-p')

    streamOperation(server('op-p', { state: 'succeeded', version: 2 }))

    await expect(done).resolves.toMatchObject({ state: 'succeeded' })
  })
})

afterEach(() => vi.unstubAllGlobals())

describe('dismissOperation', () => {
  it('asks the daemon to dismiss and keeps the dismissed row it answers', async () => {
    const failed = server('op-x', { state: 'failed', version: 3 })
    streamOperation(failed)
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

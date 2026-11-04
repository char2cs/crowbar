import { beforeEach, describe, expect, it } from 'vitest'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { useLocalOperationsStore } from '@/lib/store/local-operations'
import {
  putLocalOperation,
  selectEntityBusy,
  selectFailedOps,
  newestProblem,
  operationsState,
  type Operation,
} from '@/core/entities/operations'

function op(over: Partial<Operation>): Operation {
  return {
    id: 'op1',
    kind: 'git.push',
    subject: { kind: 'workspace', id: 'ws1' },
    projectId: 'p1',
    label: 'Push main',
    state: 'accepted',
    items: [],
    dismissed: false,
    version: 1,
    createdAt: '2026-10-28T12:00:00Z',
    updatedAt: '2026-10-28T12:00:00Z',
    ...over,
  }
}

const state = () => operationsState()
const problemOf = (entityId: string) => newestProblem(selectFailedOps(state()), [entityId])

beforeEach(() => resetOperations())

describe('operations store', () => {
  it('a frame inserts an operation and indexes it under its subject and items', () => {
    streamOperation(
      op({ items: [{ ref: { kind: 'chat', id: 'c1' }, label: 'a', state: 'running' }] }),
    )

    expect(state().ops.op1.label).toBe('Push main')
    expect(state().latestFor).toEqual({ ws1: 'op1', c1: 'op1' })
  })

  it('an older daemon reply never replaces a newer one', () => {
    putLocalOperation(op({ state: 'failed', version: 3 }))
    putLocalOperation(op({ state: 'running', version: 2 }))

    expect(state().ops.op1.state).toBe('failed')
  })

  it('a daemon reply that trails its stream frame is ignored', () => {
    streamOperation(op({ state: 'succeeded', version: 3 }))
    putLocalOperation(op({ state: 'accepted', version: 1 }))

    expect(state().ops.op1.state).toBe('succeeded')
    expect(useLocalOperationsStore.getState().ops).toEqual({})
  })

  it('a server frame replaces the local operation with the same id', () => {
    putLocalOperation(op({ version: 0, local: true, label: 'Push' }))
    streamOperation(op({ version: 1 }))

    expect(state().ops.op1.local).toBeUndefined()
    expect(state().ops.op1.label).toBe('Push main')
    putLocalOperation(op({ version: 0, local: true, state: 'failed' }))
    expect(state().ops.op1.state).toBe('accepted')
  })

  it('a reconnect never drops a local operation still in flight', () => {
    putLocalOperation(op({ id: 'mine', version: 0, local: true }))
    streamOperation(op({ id: 'other', version: 4, state: 'succeeded' }))

    expect(state().ops.mine.state).toBe('accepted')
    expect(state().ops.mine.local).toBe(true)
  })

  it('newestProblem answers the newest failed operation naming the entity, cleared by a later success', () => {
    const held = { code: 'conflict' as const, message: 'held' }
    streamOperation(op({ state: 'failed', error: held }))
    expect(problemOf('ws1')).toEqual(held)

    streamOperation(op({ id: 'op2', state: 'succeeded', createdAt: '2026-10-28T12:01:00Z' }))
    expect(problemOf('ws1')).toBeNull()
  })

  // Shallow-equal across frames that change no reason, so the sidebar does not re-derive.
  it('selectFailedOps keys each failing entity to its operation, by reference', () => {
    const held = { code: 'conflict' as const, message: 'held' }
    streamOperation(op({ state: 'failed', error: held }))
    const before = selectFailedOps(state())
    expect(before.ws1?.error).toEqual(held)

    streamOperation(op({ id: 'op9', subject: { kind: 'workspace', id: 'ws2' }, state: 'running' }))
    expect(selectFailedOps(state()).ws1).toBe(before.ws1)
    expect(selectFailedOps(state())).not.toHaveProperty('ws2')
  })

  it('newestProblem picks the newer failure among the ids a row names', () => {
    const imported = { code: 'internal' as const, message: 'not a directory' }
    const retried = { code: 'conflict' as const, message: 'still held' }
    streamOperation(
      op({
        id: 'imp',
        kind: 'branch.import',
        subject: { kind: 'repo', id: 'r1' },
        state: 'failed',
        error: { code: 'partial', message: '1 of 1 failed' },
        items: [{ ref: { kind: 'chat', id: 'c1' }, label: 'b', state: 'failed', error: imported }],
        createdAt: '2026-10-28T12:00:00Z',
      }),
    )
    expect(newestProblem(selectFailedOps(state()), ['c1', 'ws1'])).toEqual(imported)
    streamOperation(
      op({ id: 'retry', state: 'failed', error: retried, createdAt: '2026-10-28T12:05:00Z' }),
    )
    expect(newestProblem(selectFailedOps(state()), ['c1', 'ws1'])).toEqual(retried)
    expect(newestProblem(selectFailedOps(state()), [null, undefined])).toBeNull()
  })

  it('newestProblem answers a failed item error for the item ref', () => {
    const taken = { code: 'id_conflict' as const, message: 'taken' }
    streamOperation(
      op({
        state: 'failed',
        error: { code: 'partial', message: '1 of 2 failed' },
        items: [
          { ref: { kind: 'chat', id: 'c1' }, label: 'a', state: 'succeeded' },
          { ref: { kind: 'chat', id: 'c2' }, label: 'b', state: 'failed', error: taken },
        ],
      }),
    )

    expect(problemOf('c2')).toEqual(taken)
    expect(problemOf('c1')).toBeNull()
    expect(problemOf('ws1')?.code).toBe('partial')
  })

  it('selectEntityBusy is true only while an operation naming the entity is accepted or running', () => {
    streamOperation(op({ state: 'accepted' }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(true)
    expect(selectEntityBusy(state(), 'ws2')).toBe(false)

    streamOperation(op({ state: 'running', version: 2 }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(true)

    streamOperation(op({ state: 'succeeded', version: 3 }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(false)
  })

  it('the newest operation is by time, whatever the offset or precision its timestamp is written in', () => {
    streamOperation(op({ id: 'later', state: 'failed', createdAt: '2026-10-28T12:30:00Z' }))
    // 21:00+09:00 is 12:00Z, earlier, though it sorts after as text.
    streamOperation(
      op({ id: 'earlier', state: 'succeeded', createdAt: '2026-10-28T21:00:00.5+09:00' }),
    )
    expect(state().latestFor.ws1).toBe('later')

    streamOperation(op({ id: 'newest', state: 'succeeded', createdAt: '2026-10-28T12:30:00.1Z' }))
    expect(state().latestFor.ws1).toBe('newest')
  })

  it('a dismissed failure no longer names its entity a problem', () => {
    streamOperation(op({ state: 'failed', error: { code: 'internal', message: 'gone' } }))
    streamOperation(
      op({
        state: 'failed',
        version: 2,
        dismissed: true,
        error: { code: 'internal', message: 'gone' },
      }),
    )
    expect(problemOf('ws1')).toBeNull()
  })
})

import { beforeEach, describe, expect, it } from 'vitest'
import {
  applyOperationFrame,
  putLocalOperation,
  selectEntityBusy,
  selectEntityProblem,
  selectProblems,
  useOperationsStore,
  type Operation,
} from '@/lib/store/operations'

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

const state = () => useOperationsStore.getState()

beforeEach(() => useOperationsStore.setState({ ops: {}, latestFor: {} }))

describe('operations store', () => {
  it('a frame inserts an operation and indexes it under its subject and items', () => {
    applyOperationFrame(
      op({ items: [{ ref: { kind: 'chat', id: 'c1' }, label: 'a', state: 'running' }] }),
    )

    expect(state().ops.op1.label).toBe('Push main')
    expect(state().latestFor).toEqual({ ws1: 'op1', c1: 'op1' })
  })

  it('an older frame never replaces a newer one', () => {
    applyOperationFrame(op({ state: 'failed', version: 3 }))
    applyOperationFrame(op({ state: 'running', version: 2 }))

    expect(state().ops.op1.state).toBe('failed')
  })

  it('a server frame replaces the local operation with the same id', () => {
    putLocalOperation(op({ version: 0, local: true, label: 'Push' }))
    applyOperationFrame(op({ version: 1 }))

    expect(state().ops.op1.local).toBeUndefined()
    expect(state().ops.op1.label).toBe('Push main')
    putLocalOperation(op({ version: 0, local: true, state: 'failed' }))
    expect(state().ops.op1.state).toBe('accepted')
  })

  it('a reconnect never drops a local operation still in flight', () => {
    putLocalOperation(op({ id: 'mine', version: 0, local: true }))
    applyOperationFrame(op({ id: 'other', version: 4, state: 'succeeded' }))

    expect(state().ops.mine.state).toBe('accepted')
    expect(state().ops.mine.local).toBe(true)
  })

  it('selectEntityProblem returns the newest failed operation naming the entity, cleared by a later success', () => {
    const held = { code: 'conflict' as const, message: 'held' }
    applyOperationFrame(op({ state: 'failed', error: held }))
    expect(selectEntityProblem(state(), 'ws1')).toEqual(held)

    applyOperationFrame(op({ id: 'op2', state: 'succeeded', createdAt: '2026-10-28T12:01:00Z' }))
    expect(selectEntityProblem(state(), 'ws1')).toBeNull()
  })

  // Shallow-equal across frames that change no reason, so the sidebar does not re-derive.
  it('selectProblems keys each failing entity to its reason, by reference', () => {
    const held = { code: 'conflict' as const, message: 'held' }
    applyOperationFrame(op({ state: 'failed', error: held }))
    const before = selectProblems(state())
    expect(before).toEqual({ ws1: held })

    applyOperationFrame(
      op({ id: 'op9', subject: { kind: 'workspace', id: 'ws2' }, state: 'running' }),
    )
    expect(selectProblems(state()).ws1).toBe(before.ws1)
    expect(selectProblems(state())).not.toHaveProperty('ws2')
  })

  it('selectEntityProblem returns a failed item error for the item ref', () => {
    const taken = { code: 'id_conflict' as const, message: 'taken' }
    applyOperationFrame(
      op({
        state: 'failed',
        error: { code: 'partial', message: '1 of 2 failed' },
        items: [
          { ref: { kind: 'chat', id: 'c1' }, label: 'a', state: 'succeeded' },
          { ref: { kind: 'chat', id: 'c2' }, label: 'b', state: 'failed', error: taken },
        ],
      }),
    )

    expect(selectEntityProblem(state(), 'c2')).toEqual(taken)
    expect(selectEntityProblem(state(), 'c1')).toBeNull()
    expect(selectEntityProblem(state(), 'ws1')?.code).toBe('partial')
  })

  it('selectEntityBusy is true only while an operation naming the entity is accepted or running', () => {
    applyOperationFrame(op({ state: 'accepted' }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(true)
    expect(selectEntityBusy(state(), 'ws2')).toBe(false)

    applyOperationFrame(op({ state: 'running', version: 2 }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(true)

    applyOperationFrame(op({ state: 'succeeded', version: 3 }))
    expect(selectEntityBusy(state(), 'ws1')).toBe(false)
  })

  it('the newest operation is by time, whatever the offset or precision its timestamp is written in', () => {
    applyOperationFrame(op({ id: 'later', state: 'failed', createdAt: '2026-10-28T12:30:00Z' }))
    // 21:00+09:00 is 12:00Z, earlier, though it sorts after as text.
    applyOperationFrame(
      op({ id: 'earlier', state: 'succeeded', createdAt: '2026-10-28T21:00:00.5+09:00' }),
    )
    expect(state().latestFor.ws1).toBe('later')

    applyOperationFrame(
      op({ id: 'newest', state: 'succeeded', createdAt: '2026-10-28T12:30:00.1Z' }),
    )
    expect(state().latestFor.ws1).toBe('newest')
  })

  it('a dismissed failure no longer names its entity a problem', () => {
    applyOperationFrame(op({ state: 'failed', error: { code: 'internal', message: 'gone' } }))
    applyOperationFrame(
      op({
        state: 'failed',
        version: 2,
        dismissed: true,
        error: { code: 'internal', message: 'gone' },
      }),
    )
    expect(selectEntityProblem(state(), 'ws1')).toBeNull()
  })
})

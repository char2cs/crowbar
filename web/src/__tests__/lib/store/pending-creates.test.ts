import { beforeEach, describe, expect, it } from 'vitest'
import { getInitialPendingCreatesState, usePendingCreatesStore } from '@/lib/store/pending-creates'

const base = {
  tempId: 'pending-1',
  kind: 'branch' as const,
  projectId: 'p1',
  parentId: 'c-a',
  order: 2,
  workspaceId: null,
  ownsWorktree: true,
}

beforeEach(() => {
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
})

describe('a fork create in flight', () => {
  it('snapshots the panel when its request fires, not when the naming input opened', () => {
    const store = usePendingCreatesStore.getState()
    store.startNaming(base)
    expect(usePendingCreatesStore.getState().entries[0].rowIdsAtClick).toBeUndefined()

    store.confirmNaming('pending-1', 'feat/x', ['c-a', 'arrived-while-typing'])
    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      status: 'creating',
      label: 'feat/x',
      rowIdsAtClick: ['c-a', 'arrived-while-typing'],
    })
  })

  it('keeps the snapshot beside the real id once the POST answers', () => {
    const store = usePendingCreatesStore.getState()
    store.addCreating({ ...base, kind: 'chat', rowIdsAtClick: ['c-a'] })
    store.attachRealId('pending-1', 'real-1')
    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      realId: 'real-1',
      rowIdsAtClick: ['c-a'],
    })
  })
})

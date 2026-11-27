import { describe, it, expect, beforeEach } from 'vitest'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

beforeEach(() => useDetachModalStore.setState({ target: null }))

describe('detach-modal-store', () => {
  it('opens with a target and closes back to null', () => {
    useDetachModalStore.getState().open({ wsId: 'w1', branch: 'develop', heldByPath: '/repo' })
    expect(useDetachModalStore.getState().target).toEqual({
      wsId: 'w1',
      branch: 'develop',
      heldByPath: '/repo',
    })
    useDetachModalStore.getState().close()
    expect(useDetachModalStore.getState().target).toBeNull()
  })
})

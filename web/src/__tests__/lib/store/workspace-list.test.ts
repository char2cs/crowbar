import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(async () => [{ id: 'r1', name: 'repo', workspaces: [] }]),
}))

beforeEach(() => {
  resetDB()
})

describe('useWorkspaceListStore', () => {
  it('fetch loads repos into loadable', async () => {
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()
    expect(dataOf(useWorkspaceListStore.getState().data)).toEqual([
      { id: 'r1', name: 'repo', workspaces: [] },
    ])
  })
})

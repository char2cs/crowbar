import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'

vi.mock('@/lib/api', () => ({ fetchProjects: vi.fn(async () => [{ id: 'p1', name: 'P1', path: '/p1' }]) }))

beforeEach(() => { resetDB() })

describe('useProjectDataStore', () => {
  it('fetch populates loadable success', async () => {
    const { useProjectDataStore } = await import('@/lib/store/projects')
    await useProjectDataStore.getState().fetch()
    expect(useProjectDataStore.getState().data.status).toBe('success')
  })
})

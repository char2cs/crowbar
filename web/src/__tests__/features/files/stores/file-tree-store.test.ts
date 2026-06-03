import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => ({ name: 'root', children: [] })) }))

beforeEach(() => { resetDB() })

describe('useFileTreeStore', () => {
  it('fetch loads the tree for a root path', async () => {
    const { useFileTreeStore } = await import('@/features/files/stores/file-tree-store')
    await useFileTreeStore.getState().fetch('/root')
    expect(dataOf(useFileTreeStore.getState().data)).toEqual({ name: 'root', children: [] })
  })
})

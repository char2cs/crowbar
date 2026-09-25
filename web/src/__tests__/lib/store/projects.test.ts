import { describe, it, expect, vi } from 'vitest'
import { dataOf } from '@/lib/loadable'
import { importProjectAndSync, useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

vi.mock('@/lib/api', () => ({ fetchProjects: vi.fn(async () => []) }))

const mockProject: Project = {
  id: 'p1',
  name: 'my-app',
  path: '/repos/my-app',
  lastActivity: new Date(),
}

describe('importProjectAndSync', () => {
  // OOBE navigates to `/` right after an import, and that route reads this
  // list: the import's own answer must already be in it.
  it('puts the imported project in the held list at once', async () => {
    await useProjectDataStore.getState().fetch()
    importProjectAndSync(mockProject)
    expect(dataOf(useProjectDataStore.getState().data)?.map((p) => p.id)).toEqual(['p1'])
  })
})

import { describe, it, expect, beforeEach } from 'vitest'
import { useProjectStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

const mockProject: Project = {
  id: 'p1', name: 'my-app', path: '/repos/my-app', lastActivity: new Date(),
}

beforeEach(() => {
  useProjectStore.setState({ projects: [], activeProjectId: '' })
})

describe('useProjectStore', () => {
  it('starts with empty projects', () => {
    expect(useProjectStore.getState().projects).toHaveLength(0)
  })

  it('setProjects replaces the full list', () => {
    useProjectStore.getState().setProjects([mockProject])
    expect(useProjectStore.getState().projects).toHaveLength(1)
    expect(useProjectStore.getState().projects[0].id).toBe('p1')
  })

  it('addProject appends to the list', () => {
    useProjectStore.getState().setProjects([mockProject])
    const second: Project = { ...mockProject, id: 'p2', name: 'other' }
    useProjectStore.getState().addProject(second)
    expect(useProjectStore.getState().projects).toHaveLength(2)
  })
})

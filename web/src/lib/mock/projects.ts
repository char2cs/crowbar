import type { Project } from '@/lib/types'

const INITIAL_PROJECTS: Project[] = [
  { id: 'rabbyte', name: 'Rabbyte', path: '/Users/mateo/dev/rabbyte', lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000) },
  { id: 'personal', name: 'Personal', path: '/Users/mateo/dev/personal', lastActivity: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000) },
]

const store = new Map<string, Project>(INITIAL_PROJECTS.map(p => [p.id, p]))

export function getAllMockProjects(): Project[] {
  return Array.from(store.values())
}

export function getMockProject(id: string): Project | undefined {
  return store.get(id)
}

export function createMockProject(data: { name: string; path: string }): Project {
  const id = `proj-${Date.now()}`
  const project: Project = { id, ...data, lastActivity: new Date() }
  store.set(id, project)
  return project
}

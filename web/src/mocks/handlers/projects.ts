import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { createMockProject, getMockProject } from '@/lib/mock/projects'
import { ok, fail } from './envelope'

export const projectHandlers = [
  http.get('/v0/projects', ({ request }) => {
    if (shouldFault(request, 'projects')) return fail('simulated failure', 500)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.projects())
  }),

  http.get('/v0/projects/:id', ({ params }) => {
    const project = getMockProject(params.id as string)
    if (!project) return fail('not found', 404)
    return ok(project)
  }),

  // Matches the backend's WriteMutationOK: returns only { id }, not the entity.
  http.post('/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    const project = createMockProject(body)
    return ok({ id: project.id }, 201)
  }),
]

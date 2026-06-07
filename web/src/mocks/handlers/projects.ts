import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { createMockProject } from '@/lib/mock/projects'
import { ok, fail } from './envelope'

export const projectHandlers = [
  http.get('/v0/projects', ({ request }) => {
    if (shouldFault(request, 'projects')) return fail('simulated failure', 500)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.projects())
  }),

  http.post('/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    return ok(createMockProject(body), 201)
  }),
]

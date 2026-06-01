import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { createMockProject } from '@/lib/mock/projects'

export const projectHandlers = [
  http.get('/api/v0/projects', ({ request }) => {
    if (shouldFault(request, 'projects'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.projects())
  }),

  http.post('/api/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    return HttpResponse.json(createMockProject(body), { status: 201 })
  }),
]

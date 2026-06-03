import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces', ({ request }) => {
    if (shouldFault(request, 'workspaces'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.repos())
  }),

  http.get('/api/v0/workspaces/:id', ({ params, request }) => {
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.workspace(params.id as string)
    if (!ws) return HttpResponse.json({ error: 'not found' }, { status: 404 })
    return HttpResponse.json(ws)
  }),

  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string }
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.createWorkspace(body.repoId, body.branch)
    return HttpResponse.json(ws, { status: 201 })
  }),
]

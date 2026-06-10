import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { ok, fail } from './envelope'

export const workspaceHandlers = [
  http.get('/v0/workspaces', ({ request }) => {
    if (shouldFault(request, 'workspaces')) return fail('simulated failure', 500)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.repos())
  }),

  http.get('/v0/workspaces/:id', ({ params, request }) => {
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.workspace(params.id as string)
    if (!ws) return fail('not found', 404)
    return ok(ws)
  }),

  http.post('/v0/workspaces', async ({ request }) => {
    const body = (await request.json()) as { repoId: string; branch: string }
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.createWorkspace(body.repoId, body.branch)
    // Matches the backend's WriteMutationOK: returns only { id }, not the entity.
    return ok({ id: ws.id }, 201)
  }),
]

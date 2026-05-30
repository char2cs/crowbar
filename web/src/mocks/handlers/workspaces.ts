import { http, HttpResponse } from 'msw'
import { getMockWorkspace, createMockWorkspace, getMockRepos } from '@/lib/mock/workspaces'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces', () => {
    return HttpResponse.json(getMockRepos())
  }),

  http.get('/api/v0/workspaces/:id', ({ params }) => {
    const ws = getMockWorkspace(params.id as string)
    if (!ws) return HttpResponse.json({ error: 'not found' }, { status: 404 })
    return HttpResponse.json(ws)
  }),

  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string; flowName: string }
    const ws = createMockWorkspace(body.repoId, body.branch, body.flowName)
    return HttpResponse.json(ws, { status: 201 })
  }),
]

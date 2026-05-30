import { http, HttpResponse } from 'msw'
import { getAllMockProjects, createMockProject } from '@/lib/mock/projects'

export const projectHandlers = [
  http.get('/api/v0/projects', () => HttpResponse.json(getAllMockProjects())),

  http.post('/api/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    return HttpResponse.json(createMockProject(body), { status: 201 })
  }),
]

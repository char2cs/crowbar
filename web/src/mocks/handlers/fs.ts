import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const fsHandlers = [
  http.get('/api/v0/fs/tree', ({ request }) => {
    if (shouldFault(request, 'file-tree'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const root = new URL(request.url).searchParams.get('root') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.fileTree(root))
  }),

  http.get('/api/v0/fs/file', ({ request }) => {
    if (shouldFault(request, 'file-content'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const path = new URL(request.url).searchParams.get('path') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.fileContent(path))
  }),
]

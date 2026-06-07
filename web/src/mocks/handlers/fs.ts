import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { ok, fail } from './envelope'

export const fsHandlers = [
  http.get('/v0/fs/tree', ({ request }) => {
    if (shouldFault(request, 'file-tree')) return fail('simulated failure', 500)
    const root = new URL(request.url).searchParams.get('root') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.fileTree(root))
  }),

  http.get('/v0/fs/file', ({ request }) => {
    if (shouldFault(request, 'file-content')) return fail('simulated failure', 500)
    const path = new URL(request.url).searchParams.get('path') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.fileContent(path))
  }),
]

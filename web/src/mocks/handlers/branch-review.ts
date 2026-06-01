import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const branchReviewHandlers = [
  http.get('/api/v0/branch-review/:wsId/diff', ({ params, request }) => {
    if (shouldFault(request, 'branch-diff'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchDiff(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/threads', ({ params, request }) => {
    if (shouldFault(request, 'branch-threads'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchThreads(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/description', ({ params, request }) => {
    if (shouldFault(request, 'branch-description'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchDescription(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/chats', ({ params, request }) => {
    if (shouldFault(request, 'branch-chats'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchChats(params.wsId as string))
  }),
]

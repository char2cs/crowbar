import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const markdownChatHandlers = [
  http.get('/api/v0/markdown-chat/:wsId/:stepId', ({ params, request }) => {
    if (shouldFault(request, 'markdown-chat'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.markdownTurns(String(params.wsId), String(params.stepId)))
  }),
]

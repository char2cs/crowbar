import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const chatHandlers = [
  http.get('/api/v0/chats', ({ request }) => {
    if (shouldFault(request, 'chats'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const url = new URL(request.url)
    const wsId = url.searchParams.get('wsId') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.chats(wsId))
  }),
]

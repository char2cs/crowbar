import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { ok, fail } from './envelope'

export const chatHandlers = [
  http.get('/v0/chats', ({ request }) => {
    if (shouldFault(request, 'chats')) return fail('simulated failure', 500)
    const url = new URL(request.url)
    const wsId = url.searchParams.get('wsId') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.chats(wsId))
  }),
]

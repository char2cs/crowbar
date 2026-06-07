import { http } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { ok, fail } from './envelope'

export const markdownChatHandlers = [
  http.get('/v0/markdown-chat/:wsId/:stepId', ({ params, request }) => {
    if (shouldFault(request, 'markdown-chat')) return fail('simulated failure', 500)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return ok(data.markdownTurns(String(params.wsId), String(params.stepId)))
  }),
]

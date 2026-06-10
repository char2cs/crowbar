import { http } from 'msw'
import { getMockConversation } from '@/lib/mock/conversations'
import { ok } from './envelope'

export const conversationHandlers = [
  http.get('/v0/conversations/:wsId/:step', ({ params }) =>
    ok({ messages: getMockConversation(params.wsId as string, params.step as string) }),
  ),
]

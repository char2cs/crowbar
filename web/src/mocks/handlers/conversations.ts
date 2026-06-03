import { http, HttpResponse } from 'msw'
import { getMockConversation } from '@/lib/mock/conversations'

export const conversationHandlers = [
  http.get('/api/v0/conversations/:wsId/:step', ({ params }) =>
    HttpResponse.json({ messages: getMockConversation(params.wsId as string, params.step as string) })
  ),
]

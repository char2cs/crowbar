import { http, HttpResponse } from 'msw'
import { getMockMarkdownTurns } from '@/lib/mock/markdown-chat'

export const markdownChatHandlers = [
  http.get('/api/v0/markdown-chat/:wsId/:stepId', ({ params }) => {
    const turns = getMockMarkdownTurns(
      String(params.wsId),
      String(params.stepId),
    )
    return HttpResponse.json(turns)
  }),
]

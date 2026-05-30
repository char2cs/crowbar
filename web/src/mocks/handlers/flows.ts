import { http, HttpResponse } from 'msw'
import { MOCK_FLOWS } from '@/lib/mock/flows'

export const flowHandlers = [
  http.get('/api/v0/flows', () => HttpResponse.json(MOCK_FLOWS)),
]

import { http, HttpResponse } from 'msw'

export const terminalHandlers = [
  http.post('/api/v0/terminal/sessions', () =>
    HttpResponse.json({ sessionId: crypto.randomUUID() }, { status: 201 })
  ),
]

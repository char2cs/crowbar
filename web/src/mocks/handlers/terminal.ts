import { http } from 'msw'
import { ok } from './envelope'

export const terminalHandlers = [
  http.post('/v0/terminal/sessions', () =>
    ok({ sessionId: crypto.randomUUID() }, 201)
  ),
]

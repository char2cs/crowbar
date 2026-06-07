import { ws } from 'msw'

export const gitWsHandler = ws.link('/v0/ws/git').addEventListener('connection', ({ client }) => {
  client.send(JSON.stringify({ repo: 'mock', changed: false }))

  const interval = setInterval(() => {
    client.send(JSON.stringify({ repo: 'mock', changed: true }))
  }, 8000)

  client.addEventListener('close', () => clearInterval(interval))
})

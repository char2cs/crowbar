import { ws } from 'msw'

export const terminalWsHandler = ws.link('/v0/ws/terminal/:sessionId').addEventListener('connection', ({ client, params }) => {
  const sessionId = params.sessionId as string
  client.send(JSON.stringify({ sessionId, data: 'crowbar mock terminal ready\r\n$ ', isInput: false }))

  client.addEventListener('message', ({ data }) => {
    const frame = typeof data === 'string' ? JSON.parse(data) : data
    client.send(JSON.stringify({ sessionId, data: frame.data, isInput: false }))
  })
})

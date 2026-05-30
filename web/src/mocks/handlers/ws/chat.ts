import { ws } from 'msw'

export const chatWsHandler = ws.link('/api/v0/ws/chat/:chatId').addEventListener('connection', ({ client, params }) => {
  const chatId = params.chatId as string
  const words = ['Hello', ' from', ' the', ' mock', ' AI', ' server', '!']
  let i = 0
  const interval = setInterval(() => {
    if (i < words.length) {
      client.send(JSON.stringify({ chatId, content: words[i], done: false }))
      i++
    } else {
      client.send(JSON.stringify({ chatId, content: '', done: true }))
      clearInterval(interval)
    }
  }, 150)

  client.addEventListener('close', () => clearInterval(interval))
})

import { ws } from 'msw'

const MOCK_RESPONSE =
  'Great point. Let me think through this carefully.\n\n' +
  'There are several considerations here:\n\n' +
  '1. **Performance** — the current approach has O(n²) complexity\n' +
  '2. **Correctness** — edge cases around empty inputs\n' +
  '3. **Maintainability** — the code is hard to follow\n\n' +
  'My recommendation is to refactor the core loop first.'

export const chatWsHandler = ws.link('/v0/ws/chats/:chatId/stream').addEventListener('connection', ({ client }) => {
  client.addEventListener('message', () => {
    // Stream response word-by-word on receiving any message
    const words = MOCK_RESPONSE.split(' ')
    let i = 0
    const interval = setInterval(() => {
      if (i < words.length) {
        const chunk = (i === 0 ? '' : ' ') + words[i]
        client.send(JSON.stringify({ content: chunk, done: false }))
        i++
      } else {
        client.send(JSON.stringify({ content: '', done: true }))
        clearInterval(interval)
      }
    }, 40)

    client.addEventListener('close', () => clearInterval(interval))
  })
})

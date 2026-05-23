// web/src/routes/chat/$chatId.tsx
import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { ChatView } from '@/components/chat/ChatView'
import { getMockChat } from '@/lib/mock/chats'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)

  // Reset messages whenever chatId changes
  useEffect(() => {
    setMessages(getMockChat(chatId)?.messages ?? [])
  }, [chatId])

  const handleSend = (content: string, _attachments: File[]) => {
    const userMsg: ChatMessage = {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }
    setMessages(prev => [...prev, userMsg])
    setSending(true)
    simulateStream('I can help with that. What would you like to know?', (chunk) => {
      setMessages(prev => {
        const last = prev[prev.length - 1]
        if (last?.role === 'assistant' && last.id === 'streaming') {
          return [...prev.slice(0, -1), { ...last, content: last.content + chunk }]
        }
        return [...prev, {
          id: 'streaming', role: 'assistant',
          content: chunk,
          authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
          timestamp: 'just now',
        }]
      })
    }, () => {
      setMessages(prev => prev.map(m => m.id === 'streaming' ? { ...m, id: `a${Date.now()}` } : m))
      setSending(false)
    })
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder="Ask about the Rabbyte project…"
    />
  )
}

// Simulate token streaming with word-by-word delays
function simulateStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
) {
  const words = text.split(' ')
  let i = 0
  const tick = () => {
    if (i >= words.length) { onDone(); return }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    setTimeout(tick, 40)
  }
  setTimeout(tick, 400) // initial delay before first token
}

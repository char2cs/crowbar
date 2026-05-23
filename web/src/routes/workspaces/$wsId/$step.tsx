import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { getMockConversation } from '@/lib/mock/conversations'
import { ChatView } from '@/components/chat/ChatView'
import { DiffView } from '@/components/review/DiffView'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/workspaces/$wsId/$step')({
  component: StepPage,
})

function StepPage() {
  const { wsId, step } = Route.useParams()
  const { data: workspace } = useQuery(workspaceQueryOptions(wsId))

  if (!workspace) return null

  const stateDef = workspace.flow.states.find(s => s.name === step)
  const ui = stateDef?.ui ?? 'chat'

  if (ui === 'diff') {
    return <DiffView workspaceId={wsId} step={step} />
  }

  return <WorkspaceChatView workspaceId={wsId} step={step} />
}

function WorkspaceChatView({ workspaceId, step }: { workspaceId: string; step: string }) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)

  useEffect(() => {
    setMessages(getMockConversation(workspaceId, step))
  }, [workspaceId, step])

  const handleSend = (content: string) => {
    const userMsg: ChatMessage = {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }
    setMessages(prev => [...prev, userMsg])
    setSending(true)
    simulateStream(
      'Understood. I\'ll start working on that now and update you as I make progress.',
      (chunk) => {
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last?.role === 'assistant' && last.id === 'streaming') {
            return [...prev.slice(0, -1), { ...last, content: last.content + chunk }]
          }
          return [...prev, {
            id: 'streaming', role: 'assistant', content: chunk,
            authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
            timestamp: 'just now',
          }]
        })
      },
      () => {
        setMessages(prev => prev.map(m => m.id === 'streaming' ? { ...m, id: `a${Date.now()}` } : m))
        setSending(false)
      },
    )
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder={`Message… (${step})`}
    />
  )
}

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
  setTimeout(tick, 400)
}

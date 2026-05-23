import { useState } from 'react'
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
  const [messages, setMessages] = useState<ChatMessage[]>(() =>
    getMockConversation(workspaceId, step),
  )
  const [sending, setSending] = useState(false)

  const handleSend = (content: string) => {
    setMessages(prev => [...prev, {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }])
    setSending(true)
    setTimeout(() => {
      setMessages(prev => [...prev, {
        id: `a${Date.now()}`, role: 'assistant',
        content: '(Mock response — AI not yet connected)',
        authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
        timestamp: 'just now', toolCalls: 0, durationSec: 1.5,
      }])
      setSending(false)
    }, 1500)
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      modelName="Sonnet 4.6"
      tokenPct={12}
      inputPlaceholder={`Message… (${step})`}
    />
  )
}

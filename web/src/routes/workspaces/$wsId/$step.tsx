import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { useConversationStore } from '@/lib/store/conversations'
import { ChatView } from '@/components/chat/ChatView'
import { DiffView } from '@/components/review/DiffView'

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
  const [sending, setSending] = useState(false)
  const { getMessages, appendMessage, pushStreamChunk, finalizeStream } = useConversationStore()

  // Seed the store on first render for this wsId+step (no-op if already seeded)
  const messages = getMessages(workspaceId, step)

  const handleSend = (content: string, _attachments: File[]) => {
    appendMessage(workspaceId, step, {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    })
    setSending(true)
    simulateStream(
      'Understood. I\'ll start working on that now.\n\n**Next steps:**\n- Analyse the requirements\n- Draft the implementation plan\n- Write the code\n\nI\'ll update you as I make progress.',
      (chunk) => {
        pushStreamChunk(workspaceId, step, chunk, {
          role: 'assistant',
          authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
          timestamp: 'just now',
        })
      },
      () => {
        finalizeStream(workspaceId, step, `a${Date.now()}`)
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

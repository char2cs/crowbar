import { useState, useEffect, useRef, useCallback } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { nanoid } from 'nanoid'
import { workspaceQueryOptions } from '@/lib/queries'
import { useConversationStore } from '@/lib/store/conversations'
import { simulateStream } from '@/lib/mock/simulate-stream'
import { ChatView } from '@/components/chat/ChatView'
import { DiffView } from '@/components/review/DiffView'

export const Route = createFileRoute('/workspaces/$wsId/$step')({
  component: StepPage,
})

function StepPage() {
  const { wsId, step } = Route.useParams()
  const { data: workspace, isError, isPending } = useQuery(workspaceQueryOptions(wsId))

  if (isPending) return null

  if (isError || !workspace) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
        <p>Workspace not found.</p>
        <Link to="/" className="underline hover:text-foreground">Back to home</Link>
      </div>
    )
  }

  const stateDef = workspace.flow.states.find(s => s.name === step)
  const ui = stateDef?.ui ?? 'chat'

  if (ui === 'diff') return <DiffView workspaceId={wsId} step={step} />
  return <WorkspaceChatView workspaceId={wsId} step={step} />
}

function WorkspaceChatView({ workspaceId, step }: { workspaceId: string; step: string }) {
  const [sending, setSending] = useState(false)
  const cancelStreamRef = useRef<(() => void) | null>(null)
  const { getMessages, appendMessage, pushStreamChunk, finalizeStream } = useConversationStore()

  // Cancel in-flight stream when workspaceId or step changes
  useEffect(() => {
    return () => { cancelStreamRef.current?.() }
  }, [workspaceId, step])

  // Cancel in-flight stream on unmount
  useEffect(() => {
    return () => { cancelStreamRef.current?.() }
  }, [])

  const messages = getMessages(workspaceId, step)

  const handleSend = useCallback((content: string, _attachments: File[]) => {
    cancelStreamRef.current?.()

    appendMessage(workspaceId, step, {
      id: nanoid(), role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    })
    setSending(true)

    cancelStreamRef.current = simulateStream(
      'Understood. I\'ll start working on that now.\n\n**Next steps:**\n- Analyse the requirements\n- Draft the implementation plan\n- Write the code\n\nI\'ll update you as I make progress.',
      (chunk) => {
        pushStreamChunk(workspaceId, step, chunk, {
          role: 'assistant',
          authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
          timestamp: 'just now',
        })
      },
      () => {
        finalizeStream(workspaceId, step, nanoid())
        setSending(false)
        cancelStreamRef.current = null
      },
    )
  }, [workspaceId, step, appendMessage, pushStreamChunk, finalizeStream])

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder={`Message… (${step})`}
    />
  )
}

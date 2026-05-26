import { useEffect, useRef, useCallback } from 'react'
import { nanoid } from 'nanoid'
import { ChatView as BaseChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'
import { simulateStream } from '@/lib/mock/simulate-stream'
import { useChatMessages, useIsSending, useWorkflowActions } from '@/features/workspace/stores/hooks/use-workflow'

interface ChatViewProps {
  workspaceId: string
  stepId: string
}

const STEP_PLACEHOLDERS: Record<string, string> = {
  brainstorm: 'Describe the feature or problem you want to solve…',
  spec: 'Ask about the spec, request changes, or refine requirements…',
  build: 'Ask for help with the implementation…',
  ai_review: 'Ask about the review findings or request changes…',
  human_review: 'Leave review comments or feedback…',
}

const STEP_INITIAL_MESSAGES: Record<string, ChatMessage[]> = {
  brainstorm: [
    {
      id: 'bs-1',
      role: 'assistant',
      content: "I'm here to help you brainstorm. What feature or problem do you want to tackle in this workspace?",
      authorName: 'Claude',
      authorInitials: '✦',
      modelName: 'Sonnet 4.6',
      timestamp: 'just now',
    },
  ],
  spec: [],
  build: [],
  ai_review: [],
  human_review: [],
}

const STREAMING_ID = 'streaming'

export function ChatView({ workspaceId: _workspaceId, stepId }: ChatViewProps) {
  const messages = useChatMessages(stepId)
  const sending = useIsSending(stepId)
  const cancelStreamRef = useRef<(() => void) | null>(null)
  const {
    initChatStep,
    addChatMessage,
    appendStreamingChunk,
    finaliseStreamingMessage,
    setSendingForStep,
  } = useWorkflowActions()

  // Seed initial messages the first time this step is rendered.
  useEffect(() => {
    initChatStep(stepId, STEP_INITIAL_MESSAGES[stepId] ?? [])
  }, [stepId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Cancel any in-flight stream when stepId changes or component unmounts.
  useEffect(() => {
    return () => { cancelStreamRef.current?.() }
  }, [stepId])

  const handleSend = useCallback((content: string, _attachments: File[]) => {
    cancelStreamRef.current?.()

    const userMsg: ChatMessage = {
      id: nanoid(),
      role: 'user',
      content,
      authorName: 'Mateo',
      authorInitials: 'MU',
      timestamp: 'just now',
    }
    addChatMessage(stepId, userMsg)
    setSendingForStep(stepId, true)

    cancelStreamRef.current = simulateStream(
      "Great idea. Let me think through this with you. I can help refine the approach, identify edge cases, and suggest implementation strategies.",
      (chunk) => {
        appendStreamingChunk(stepId, chunk, STREAMING_ID)
      },
      () => {
        finaliseStreamingMessage(stepId, STREAMING_ID, nanoid())
        setSendingForStep(stepId, false)
        cancelStreamRef.current = null
      },
    )
  }, [stepId, addChatMessage, appendStreamingChunk, finaliseStreamingMessage, setSendingForStep])

  return (
    <BaseChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder={STEP_PLACEHOLDERS[stepId] ?? 'Message…'}
    />
  )
}

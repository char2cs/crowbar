import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'
import { ChatEmptyState } from './ChatEmptyState'
import type { ChatMessage } from '@/lib/types'

export type { ChatMessage }

interface ChatViewProps {
  messages: ChatMessage[]
  onSend: (content: string, attachments: File[]) => void
  inputPlaceholder?: string
  sending?: boolean
}

export function ChatView({
  messages,
  onSend,
  inputPlaceholder = 'Message…',
  sending,
}: ChatViewProps) {
  const bottomRef = useRef<HTMLDivElement>(null)
  // True when the bottom sentinel is visible — user is at (or near) the bottom.
  const isAtBottomRef = useRef(true)

  // Track whether the bottom sentinel is visible. When the user scrolls up,
  // the sentinel exits the scroll area's clipping region and isIntersecting becomes false.
  // Guard for environments (e.g. jsdom) that don't implement IntersectionObserver.
  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') return
    const sentinel = bottomRef.current
    if (!sentinel) return
    const observer = new IntersectionObserver(
      ([entry]) => { isAtBottomRef.current = entry.isIntersecting },
      { threshold: 0.1 },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [])

  // Auto-scroll only when the user is already at the bottom.
  useEffect(() => {
    if (isAtBottomRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages])

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-6">
          {messages.length === 0 && !sending ? (
            <ChatEmptyState />
          ) : (
            messages.map((msg, i) => (
              <div key={msg.id}>
                {i > 0 &&
                  msg.role === 'assistant' &&
                  messages[i - 1].role === 'user' &&
                  msg.toolCalls !== undefined &&
                  msg.durationSec !== undefined && (
                    <ToolCallSeparator toolCalls={msg.toolCalls} durationSec={msg.durationSec} />
                  )}
                <MessageBubble
                  role={msg.role}
                  content={msg.content}
                  authorName={msg.authorName}
                  authorInitials={msg.authorInitials}
                  modelName={msg.modelName}
                  timestamp={msg.timestamp}
                  isStreaming={msg.id === 'streaming'}
                />
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
      <ChatInput
        placeholder={inputPlaceholder}
        onSend={onSend}
        disabled={sending}
      />
    </div>
  )
}

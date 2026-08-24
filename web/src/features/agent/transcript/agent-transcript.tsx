import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentActivity, AgentChatMessage, AgentProvider } from '@/features/agent/api/agent-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { WorkingLine } from '@/features/agent/activity/working-line'
import { useTranscriptAnchor } from '@/features/agent/hooks/use-transcript-anchor'
import { CompactionDivider } from '@/features/agent/transcript/compaction-divider'
import { MessageRow } from '@/features/agent/transcript/message-row'
import { QueuedRow } from '@/features/agent/transcript/queued-row'
import { AgentTurnTools } from '@/features/agent/transcript/turn-tools'

interface AgentTranscriptProps {
  messages: AgentChatMessage[]
  streamingBubble?: AgentChatMessage
  queue: PromptQueueItem[]
  providers: AgentProvider[]
  activity: AgentActivity
  working: boolean
  loading: boolean
  error: Error | null
  hasOlder: boolean
  /** A message the composer is already showing, so the transcript does not say
   *  the same sentence twice. */
  suppressSequence?: number
  /** The compaction boundary, keyed by the sequence of the first message AFTER
   *  it — the row the divider is drawn above. A compaction is a boundary
   *  BETWEEN two messages, so the message that follows it is the only one that
   *  identifies it unambiguously. */
  compactionBefore?: Record<number, 'manual' | 'auto' | string>
  onLoadOlder: () => void
  onRetryLoad: () => void
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour, if any. */
  showTerminalHintFor?: string
}

/** The provider of the nearest earlier assistant message, for the label. */
function previousAssistantProvider(messages: AgentChatMessage[], index: number): string {
  for (let i = index - 1; i >= 0; i--) {
    if (messages[i]?.role === 'assistant') return messages[i]?.providerId ?? ''
  }
  return ''
}

/**
 * The conversation.
 *
 * Bottom-anchored, because a chat is read from its newest end. Queued prompts sit
 * below the record and above the working line, which is the order they will
 * actually happen in.
 */
export function AgentTranscript(props: AgentTranscriptProps) {
  const { messages, queue } = props
  const anchor = useTranscriptAnchor()

  return (
    <div
      className="scroll"
      data-testid="agent-message-list"
      ref={anchor.scrollRef}
      onScroll={anchor.onScroll}
    >
      <div className="center stream">
        {props.hasOlder && (
          <Button
            className="self-center"
            variant="ghost"
            size="sm"
            onClick={() => {
              anchor.preservePosition()
              props.onLoadOlder()
            }}
          >
            Load earlier messages
          </Button>
        )}
        {props.loading && messages.length === 0 && (
          <div className="flex flex-1 items-center justify-center gap-2 text-muted-foreground text-sm">
            <FlickerSpinner className="size-4" /> Loading messages…
          </div>
        )}
        {props.error && messages.length === 0 && (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
            <p>Couldn’t load this chat’s messages.</p>
            <div className="flex gap-2">
              <Button size="sm" variant="secondary" onClick={props.onRetryLoad}>
                Retry
              </Button>
              <Button size="sm" variant="ghost" onClick={props.onOpenTerminal}>
                <TerminalIcon /> Terminal
              </Button>
            </div>
          </div>
        )}
        {messages.map((message, index) => (
          <div key={message.sequence}>
            {message.sequence === props.suppressSequence ? null : (
              <>
                {props.compactionBefore?.[message.sequence] && (
                  <CompactionDivider trigger={props.compactionBefore[message.sequence]} />
                )}
                <MessageRow
                  message={message}
                  providers={props.providers}
                  showProvider={
                    message.role === 'assistant' &&
                    (previousAssistantProvider(messages, index) === '' ||
                      previousAssistantProvider(messages, index) !== message.providerId)
                  }
                />
                {message.role === 'assistant' && (
                  <AgentTurnTools activity={props.activity} turnId={message.turnId ?? ''} />
                )}
              </>
            )}
          </div>
        ))}
        {props.streamingBubble && (
          <MessageRow
            message={props.streamingBubble}
            providers={props.providers}
            showProvider={false}
          />
        )}
        {queue.map((item) => (
          <QueuedRow
            key={item.clientRequestId}
            item={item}
            onEdit={() => props.onEditPrompt(item)}
            onCancel={() => props.onCancelPrompt(item.clientRequestId)}
            onRetry={() => props.onRetryPrompt(item.clientRequestId)}
            showTerminalHint={props.showTerminalHintFor === item.clientRequestId}
            onOpenTerminal={props.onOpenTerminal}
          />
        ))}
        <WorkingLine
          activity={props.activity}
          working={props.working}
          since={messages.at(-1)?.at}
        />
      </div>
    </div>
  )
}

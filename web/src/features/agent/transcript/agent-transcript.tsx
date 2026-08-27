import { useMemo } from 'react'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentActivity, AgentChatMessage, AgentProvider } from '@/features/agent/api/agent-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { WorkingLine } from '@/features/agent/activity/working-line'
import { useTranscriptAnchor } from '@/features/agent/hooks/use-transcript-anchor'
import { useScrollFrameSpan } from '@/features/agent/hooks/use-scroll-frame-span'
import { CompactionDivider } from '@/features/agent/transcript/compaction-divider'
import { FirstTurnDivider } from '@/features/agent/transcript/first-turn-divider'
import { InterruptedDivider } from '@/features/agent/transcript/interrupted-divider'
import { MessageRow } from '@/features/agent/transcript/message-row'
import { QueuedRow } from '@/features/agent/transcript/queued-row'
import { AgentTurnTools, groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'

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
  /** A person stopped the chat's first turn after it had already dispatched.
   *  LOCAL for this session only — see interrupted-divider.tsx for why. Drawn
   *  once the turn has actually gone idle, in the same slot the working line
   *  just vacated, never alongside it. */
  firstTurnInterrupted?: boolean
  onLoadOlder: () => void
  onRetryLoad: () => void
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour, if any. */
  showTerminalHintFor?: string
}

/** Sequences of assistant messages that should carry the provider label — the
 *  first assistant message in the loaded window, and any whose provider
 *  differs from the nearest earlier assistant message. One forward pass,
 *  computed once per messages change, replacing a backward walk that used to
 *  run twice per assistant row. */
function providerLabelSequences(messages: AgentChatMessage[]): Set<number> {
  const sequences = new Set<number>()
  let previousProvider = ''
  for (const message of messages) {
    if (message.role !== 'assistant') continue
    if (previousProvider === '' || previousProvider !== message.providerId) {
      sequences.add(message.sequence)
    }
    previousProvider = message.providerId ?? ''
  }
  return sequences
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
  const scrollFrame = useScrollFrameSpan()
  const callsByTurn = useMemo(
    () => groupToolCallsByTurn(props.activity.toolCalls),
    [props.activity.toolCalls],
  )
  const providerLabelSeqs = useMemo(() => providerLabelSequences(messages), [messages])
  // The ABSOLUTE first turn, never the first one merely loaded — `hasOlder`
  // paging in more history must not retroactively unfreeze a message that was
  // never actually the beginning of the conversation. Only meaningful once
  // there is nothing earlier to page in.
  const firstTurnSequence = !props.hasOlder ? messages[0]?.sequence : undefined

  return (
    <div
      className="scroll"
      data-testid="agent-message-list"
      ref={anchor.scrollRef}
      onScroll={() => {
        anchor.onScroll()
        scrollFrame.onScrollEvent()
      }}
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
        {messages.map((message) => (
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
                    message.role === 'assistant' && providerLabelSeqs.has(message.sequence)
                  }
                  firstTurn={message.sequence === firstTurnSequence}
                />
                {message.role === 'assistant' && (
                  <AgentTurnTools callsByTurn={callsByTurn} turnId={message.turnId ?? ''} />
                )}
                {message.sequence === firstTurnSequence && <FirstTurnDivider />}
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
        {props.firstTurnInterrupted && !props.working && <InterruptedDivider />}
        <WorkingLine
          activity={props.activity}
          working={props.working}
          since={messages.at(-1)?.at}
        />
      </div>
    </div>
  )
}

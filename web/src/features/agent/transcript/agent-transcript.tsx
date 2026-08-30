import { Fragment, useMemo } from 'react'
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
import { groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'

interface AgentTranscriptProps {
  messages: AgentChatMessage[]
  /** One per still-open message item — see useChatMessages. Almost always
   *  0 or 1 entries; more than one only for a provider (Codex) that can
   *  split a turn's reply across several concurrent items. */
  streamingBubbles?: AgentChatMessage[]
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
  /** A stopped turn, keyed the same way compactionBefore is: by the sequence of
   *  the first message AFTER it, the row the divider draws above. */
  interruptedBefore?: Record<number, true>
  /** The most recent stop with no later CONFIRMED message loaded yet — nothing
   *  to key it before, so it draws right after the last confirmed/streaming
   *  content instead: above any still-queued prompt too, which has no
   *  sequence yet and so can never anchor `interruptedBefore` itself. */
  trailingInterruption?: boolean
  onLoadOlder: () => void
  onRetryLoad: () => void
  onOpenTerminal: () => void
  onEditPrompt: (item: PromptQueueItem) => void
  onCancelPrompt: (id: string) => void
  onRetryPrompt: (id: string) => void
  /** The one queued row allowed to offer the terminal detour, if any. */
  showTerminalHintFor?: string
}

/** The `at` of the user turn each assistant reply actually answers, keyed by
 *  the reply's own sequence — what the turnbar times its reply AGAINST,
 *  since "how long the agent took" means the gap from the prompt, not from
 *  now. One forward pass: the most recent user message seen so far is the
 *  one every assistant message until the next user message is answering. */
function precedingUserAtByAssistantSequence(messages: AgentChatMessage[]): Map<number, string> {
  const map = new Map<number, string>()
  let lastUserAt: string | undefined
  for (const message of messages) {
    if (message.role === 'user') lastUserAt = message.at
    else if (message.role === 'assistant' && lastUserAt !== undefined) {
      map.set(message.sequence, lastUserAt)
    }
  }
  return map
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
  const precedingUserAt = useMemo(() => precedingUserAtByAssistantSequence(messages), [messages])
  // The ABSOLUTE first turn, never the first one merely loaded — `hasOlder`
  // paging in more history must not retroactively unfreeze a message that was
  // never actually the beginning of the conversation. Only meaningful once
  // there is nothing earlier to page in.
  const firstTurnSequence = !props.hasOlder ? messages[0]?.sequence : undefined
  // The assistant's answer to that frozen turn — kept in the same larger
  // hand rather than dropping to ordinary reply prose the instant the turn
  // ends. Tied to `firstTurnSequence` existing at all, not merely to being
  // the first assistant message loaded — same reasoning as above.
  const firstReplySequence =
    firstTurnSequence !== undefined
      ? messages.find((m) => m.role === 'assistant' && m.sequence > firstTurnSequence)?.sequence
      : undefined

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
                {props.interruptedBefore?.[message.sequence] && <InterruptedDivider />}
                <MessageRow
                  message={message}
                  providers={props.providers}
                  firstTurn={message.sequence === firstTurnSequence}
                  firstReply={message.sequence === firstReplySequence}
                  toolCallsByTurn={message.role === 'assistant' ? callsByTurn : undefined}
                  precedingUserAt={precedingUserAt.get(message.sequence)}
                />
                {message.sequence === firstTurnSequence && <FirstTurnDivider />}
              </>
            )}
          </div>
        ))}
        {props.trailingInterruption && !props.working && <InterruptedDivider />}
        {props.streamingBubbles?.map((bubble) => (
          <MessageRow
            key={bubble.sequence}
            message={bubble}
            providers={props.providers}
            streaming
          />
        ))}
        {queue.map((item, index) => {
          // The ABSOLUTE first turn, exactly as firstTurnSequence reasons about
          // it above: nothing loaded yet, nothing older to page in, and this is
          // the head of the queue — the one prompt that is about to BECOME
          // sequence zero, not merely the first one currently waiting.
          const isFirstTurn = index === 0 && messages.length === 0 && !props.hasOlder
          // A Fragment, NOT a div: `.queued` aligns itself via `align-self:
          // flex-end` on the assumption that it is a DIRECT flex child of
          // `.stream` — a wrapping element (even an empty-looking one) makes
          // it a normal block instead, which drops the shrink-to-fit sizing
          // flex items get and stretches it to `max-width: 88%` every time,
          // left-aligned inside that. A Fragment adds no box, so `.queued`
          // stays a real flex child same as it was before this needed a
          // sibling to hold the divider.
          return (
            <Fragment key={item.clientRequestId}>
              <QueuedRow
                item={item}
                firstTurn={isFirstTurn}
                onEdit={() => props.onEditPrompt(item)}
                onCancel={() => props.onCancelPrompt(item.clientRequestId)}
                onRetry={() => props.onRetryPrompt(item.clientRequestId)}
                showTerminalHint={props.showTerminalHintFor === item.clientRequestId}
                onOpenTerminal={props.onOpenTerminal}
              />
              {isFirstTurn && <FirstTurnDivider />}
            </Fragment>
          )
        })}
        <WorkingLine
          activity={props.activity}
          working={props.working}
          since={messages.at(-1)?.at}
        />
      </div>
    </div>
  )
}

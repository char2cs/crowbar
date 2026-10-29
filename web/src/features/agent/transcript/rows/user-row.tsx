import { memo } from 'react'
import { cn } from '@/lib/utils'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { turnTimestampLabel, turnTimeTitle } from '@/features/agent/lib/turn-time'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { UserItem } from '@/features/agent/stores/lib/transcript-items'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

/** True while the queue row still speaks for this prompt: until its turn takes it. */
function useQueued(chatId: string, item: UserItem): boolean {
  return useChatStore((s) => {
    if (item.origin !== 'crowbar' || !item.requestId) return false
    const state = s.feeds[chatId]?.facts.prompts[item.requestId]?.state
    return state === 'pending' || state === 'refused' || state === 'retracted'
  })
}

/**
 * A prompt in the record. A harness message is the provider's machinery talking
 * to its model and a note is the provider's own words about the chat: neither
 * wears the user's bubble, since nobody here typed them.
 */
function UserRowBody({
  chatId,
  item,
  providers,
  provider,
  firstTurn,
}: {
  chatId: string
  item: UserItem
  providers: AgentProvider[]
  provider?: string
  firstTurn: boolean
}) {
  const queued = useQueued(chatId, item)
  if (queued) return null
  if (item.origin === 'harness' || item.origin === 'note') {
    const harness = item.origin === 'harness'
    const name = providers.find((p) => p.id === provider)?.displayName ?? provider ?? 'the provider'
    const big = item.text.length > 160 || item.text.split('\n').length > 3
    return (
      <article className="row" data-row-id={item.id} data-role={harness ? 'harness' : 'notice'}>
        <div className={harness ? 'harness' : 'notice'} role={harness ? undefined : 'alert'}>
          {harness && (
            <p className="meta" data-testid="message-harness-label">
              Sent to the agent by {name} — not by you
            </p>
          )}
          {!harness ? (
            <span>{item.text}</span>
          ) : big ? (
            <details>
              <summary>Show payload</summary>
              <pre>
                <code>{item.text}</code>
              </pre>
            </details>
          ) : (
            <code>{item.text}</code>
          )}
        </div>
      </article>
    )
  }
  const multi = item.text.length > 60 || item.text.includes('\n')
  return (
    <article
      className={cn('row stacked', !firstTurn && 'me')}
      data-row-id={item.id}
      data-role="user"
      data-first-turn={firstTurn ? 'true' : undefined}
    >
      <div className={cn(firstTurn ? 'frozen' : 'bubble', !firstTurn && multi && 'multi')}>
        <MarkdownMessageStatic diagrams className="break-words">
          {item.text}
        </MarkdownMessageStatic>
      </div>
      <time className="turn-time" dateTime={item.at} title={turnTimeTitle(item.at)}>
        {turnTimestampLabel(item.at)}
      </time>
    </article>
  )
}

export const UserRow = memo(UserRowBody)

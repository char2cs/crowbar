import { memo } from 'react'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { turnTimestampLabel, turnTimeTitle } from '@/features/agent/lib/turn-time'
import { CloseIcon, PencilIcon } from '@/features/agent/shared/agent-icons'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { SESSION_LOST_TEXT } from '@/features/agent/stores/lib/outbox'
import type { UserItem } from '@/features/agent/stores/lib/transcript-items'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

/** What a person can do about a prompt that has not gone through. */
export interface PromptActions {
  retry: (requestId: string) => void
  /** Puts the text back in the composer; an unsent prompt leaves the outbox. */
  edit: (requestId: string, text: string) => void
  cancel: (requestId: string) => void
}

type Unsent = 'queued' | 'sending' | 'pending' | 'failed' | 'refused'

interface PromptState {
  text?: string
  state?: Unsent
  error?: string
}

/** One prompt's state, from the outbox until its row lands and from the rows after. */
function usePromptState(chatId: string, id: string, item?: UserItem): PromptState {
  const requestId = item ? item.requestId : id.slice(2)
  const queued = useChatStore((s) =>
    item ? undefined : s.outbox[chatId]?.find((entry) => entry.requestId === requestId),
  )
  const fact = useChatStore((s) =>
    requestId && item?.origin === 'crowbar' ? s.feeds[chatId]?.facts.prompts[requestId] : undefined,
  )
  if (queued) return { text: queued.text, state: queued.state, error: queued.error }
  if (fact?.state === 'pending') return { text: item?.text, state: 'pending' }
  if (fact?.state !== 'refused') return { text: item?.text }
  const error =
    fact.reason === 'session_lost'
      ? SESSION_LOST_TEXT
      : 'The provider never took this prompt. Retry sends it again.'
  return { text: item?.text, state: 'refused', error }
}

function MachineRow({ item, name }: { item: UserItem; name: string }) {
  const harness = item.origin === 'harness'
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

function PromptActs({
  requestId,
  text,
  state,
  actions,
}: {
  requestId: string
  text: string
  state: Unsent
  actions: PromptActions
}) {
  const bad = state === 'failed' || state === 'refused'
  if (!bad && state !== 'queued') return null
  return (
    <span className="acts">
      {bad && (
        <Button
          size="icon-sm"
          variant="ghost"
          tooltip="Retry"
          aria-label="Retry prompt"
          onClick={() => actions.retry(requestId)}
        >
          <ArrowCounterClockwiseIcon />
        </Button>
      )}
      <Button
        size="icon-sm"
        variant="ghost"
        tooltip="Edit"
        aria-label="Edit prompt"
        onClick={() => actions.edit(requestId, text)}
      >
        <PencilIcon />
      </Button>
      {state !== 'refused' && (
        <Button
          size="icon-sm"
          variant="ghost"
          tooltip="Cancel"
          aria-label="Cancel prompt"
          onClick={() => actions.cancel(requestId)}
        >
          <CloseIcon />
        </Button>
      )}
    </span>
  )
}

/**
 * A prompt, from Enter to its settle, under the one key `p:<requestId>`. A
 * harness message or a provider note is not the user's, so it wears no bubble.
 */
function UserRowBody({
  chatId,
  id,
  item,
  providers,
  provider,
  firstTurn,
  actions,
}: {
  chatId: string
  id: string
  /** Absent while the prompt is only in the outbox. */
  item?: UserItem
  providers: AgentProvider[]
  provider?: string
  firstTurn: boolean
  actions: PromptActions
}) {
  const { text, state, error } = usePromptState(chatId, id, item)
  if (item && (item.origin === 'harness' || item.origin === 'note')) {
    const name = providers.find((p) => p.id === provider)?.displayName ?? provider ?? 'the provider'
    return <MachineRow item={item} name={name} />
  }
  if (text === undefined) return null
  const multi = text.length > 60 || text.includes('\n')
  const bad = state === 'failed' || state === 'refused'
  return (
    <article
      className={cn('row stacked', !firstTurn && 'me')}
      data-row-id={id}
      data-role="user"
      data-state={state}
      data-testid={state ? 'queued-prompt' : undefined}
      data-first-turn={firstTurn ? 'true' : undefined}
    >
      {state ? (
        <div className={firstTurn ? 'frozen firstq' : cn('queued', multi && 'multi', bad && 'bad')}>
          <MarkdownMessageStatic className={firstTurn ? undefined : 'txt'}>
            {text}
          </MarkdownMessageStatic>
          {error && <span className="err">{error}</span>}
          <PromptActs requestId={id.slice(2)} text={text} state={state} actions={actions} />
        </div>
      ) : (
        <div className={cn(firstTurn ? 'frozen' : 'bubble', !firstTurn && multi && 'multi')}>
          <MarkdownMessageStatic diagrams className="break-words">
            {text}
          </MarkdownMessageStatic>
        </div>
      )}
      {item && !state && (
        <time className="turn-time" dateTime={item.at} title={turnTimeTitle(item.at)}>
          {turnTimestampLabel(item.at)}
        </time>
      )}
    </article>
  )
}

export const UserRow = memo(UserRowBody)

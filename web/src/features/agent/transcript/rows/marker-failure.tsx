import { useState } from 'react'
import { Button } from '@/components/ui/button'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import { limitResetsAt, useChatTelemetry } from '@/features/agent/hooks/use-chat-entity'
import {
  describeFailure,
  resetLabel,
  warningHeadline,
  type TurnFailure,
} from '@/features/agent/transcript/lib/turn-failure'
import { FailureNotice } from '@/features/agent/transcript/rows/failure-notice'

/** What a marker row can do: re-send a failed turn, open the CLI, start over. */
export interface MarkerActions {
  /** Sends the prompt `requestId` again, as its retry. */
  retry: (requestId: string) => void
  openTerminal: () => void
  /** The next prompt starts a fresh conversation. */
  newConversation: () => void
}

export interface MarkerProps {
  chatId: string
  item: MarkerItem
}

/** True while nothing was asked after the marker: no later prompt in the
 *  transcript or on its way. */
function useNewestFailure(chatId: string, id: string): boolean {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    if (!feed || (s.outbox[chatId]?.length ?? 0) > 0) return false
    const at = feed.order.indexOf(id)
    // A prompt the provider never took is no answer to this failure: it is its casualty.
    const sent = (later: string) => {
      const item = feed.items.get(later)
      if (item?.kind !== 'user') return false
      return !item.requestId || feed.facts.prompts[item.requestId]?.state !== 'refused'
    }
    return at >= 0 && !feed.order.slice(at + 1).some(sent)
  })
}

/** The request id of the prompt that started `turnId`. */
function usePromptOf(chatId: string, turnId: string | undefined): string | undefined {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    if (!feed || !turnId) return undefined
    for (const id of feed.order) {
      const item = feed.items.get(id)
      if (item?.kind === 'user' && item.turnId === turnId) return item.requestId
    }
    return undefined
  })
}

/** Sends the prompt that started the turn again, once, while nothing was asked since. */
export function RetryButton({
  chatId,
  item,
  actions,
  primary = false,
}: MarkerProps & { actions: MarkerActions; primary?: boolean }) {
  const [sent, setSent] = useState(false)
  const requestId = usePromptOf(chatId, item.turnId)
  const newest = useNewestFailure(chatId, item.id)
  if (requestId === undefined || !newest) return null
  return (
    <Button
      size="sm"
      variant={primary ? 'default' : 'secondary'}
      disabled={sent}
      onClick={() => {
        setSent(true)
        actions.retry(requestId)
      }}
    >
      Retry
    </Button>
  )
}

/** The second line from the newest failure: the sign-in to run, or when a limit lifts. */
function useHint(chatId: string, failure: TurnFailure, newest: boolean, provider?: AgentProvider) {
  const telemetry = useChatTelemetry(chatId)
  if (!newest) return failure.kind === 'limit' ? undefined : failure.hint
  if (failure.kind === 'auth' && provider?.signIn) {
    return (
      <>
        Run <code>{provider.signIn}</code> to sign in again.
      </>
    )
  }
  if (failure.kind !== 'limit') return failure.hint
  const resetsAt = limitResetsAt(telemetry)
  return resetsAt ? resetLabel(resetsAt) || undefined : undefined
}

/** A failed turn: what went wrong in plain words, with Retry as the primary way on. */
export function FailedNotice({
  chatId,
  item,
  actions,
  provider,
}: MarkerProps & { actions: MarkerActions; provider?: AgentProvider }) {
  const newest = useNewestFailure(chatId, item.id)
  const failure = describeFailure(item.reason, item.detail)
  const hint = useHint(chatId, failure, newest, provider)
  return (
    <FailureNotice
      kind={failure.kind}
      headline={failure.headline}
      hint={hint}
      detail={failure.more}
      headlineTestId="agent-failed-reason"
      actions={
        <>
          <Button size="sm" variant="outline" onClick={actions.openTerminal}>
            Open terminal
          </Button>
          <RetryButton chatId={chatId} item={item} actions={actions} primary />
        </>
      }
    />
  )
}

/** A provider's warning as a one-line notice; its whole message is behind Details. */
export function WarningNotice({ detail }: { detail?: string }) {
  const { text, more } = warningHeadline(detail)
  return (
    <FailureNotice
      kind="warning"
      headline={text || 'The agent warned of a problem'}
      detail={more}
      headlineTestId="agent-warning"
    />
  )
}

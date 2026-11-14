import { useState } from 'react'
import { Button } from '@/components/ui/button'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { limitResetsAt, useChatTelemetry } from '@/features/agent/hooks/use-chat-entity'
import { openModelPicker } from '@/features/agent/controls/model-picker-open'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import { markerFailure, type FailureActions } from '@/features/agent/transcript/lib/marker-failure'
import { FailureNotice } from '@/features/agent/transcript/rows/failure-notice'

const MARKERS_THAT_STOP = new Set([
  'turn_failed',
  'session_lost',
  'runner_exited',
  'turn_interrupted',
])

/** The newest marker that stopped the chat, while nothing was asked after it: no later
 *  prompt in the transcript or on its way. */
export function useNewestStop(chatId: string): MarkerItem | undefined {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    if (!feed || (s.outbox[chatId]?.length ?? 0) > 0) return undefined
    for (let at = feed.order.length - 1; at >= 0; at--) {
      const item = feed.items.get(feed.order[at] ?? '')
      if (item?.kind === 'user') {
        // A prompt the provider never took is no answer to a stop: it is its casualty.
        const refused = item.requestId && feed.facts.prompts[item.requestId]?.state === 'refused'
        if (!refused) return undefined
      }
      // The newest stop decides: one the person made themselves leaves nothing to say.
      if (item?.kind === 'marker' && MARKERS_THAT_STOP.has(item.marker)) {
        return markerFailure(item) ? item : undefined
      }
    }
    return undefined
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

/** Sends the prompt that started the turn again, once. */
function RetryButton({ requestId, retry }: { requestId: string; retry: FailureActions['retry'] }) {
  const [sent, setSent] = useState(false)
  return (
    <Button
      size="sm"
      disabled={sent}
      onClick={() => {
        setSent(true)
        retry(requestId)
      }}
    >
      Retry
    </Button>
  )
}

const hasCatalogue = (providers: AgentProvider[]) =>
  providers.some((p) => p.connected && p.enabled && p.modelSelect && (p.models?.length ?? 0) > 0)

/** The chat's latest stop, above the input box, with the one way on the person has. */
export function TurnFailureTray({
  chatId,
  item,
  providers,
  actions,
}: {
  chatId: string
  item: MarkerItem
  providers: AgentProvider[]
  actions: FailureActions
}) {
  const telemetry = useChatTelemetry(chatId)
  // The provider the stopped turn ran on.
  const provider = useChatStore((s) =>
    item.turnId ? s.feeds[chatId]?.facts.providers[item.turnId] : undefined,
  )
  const requestId = usePromptOf(chatId, item.turnId)
  const name = providers.find((p) => p.id === provider)?.displayName
  const failure = markerFailure(item, { provider: name, resetsAt: limitResetsAt(telemetry) })
  if (!failure) return null
  return (
    <FailureNotice
      tone={failure.tone}
      headline={failure.headline}
      hint={failure.hint}
      headlineTestId="agent-failed-reason"
      actions={
        <>
          {failure.way === 'switch-model' && hasCatalogue(providers) && (
            <Button
              size="sm"
              variant="outline"
              className="bg-background"
              onClick={() => openModelPicker(chatId)}
            >
              Switch model
            </Button>
          )}
          {failure.way === 'sign-in' && (
            <Button size="sm" onClick={actions.openTerminal}>
              Sign in
            </Button>
          )}
          {failure.way === 'new-conversation' && (
            <Button size="sm" onClick={actions.newConversation}>
              New conversation
            </Button>
          )}
          {failure.way === 'retry' && requestId !== undefined && (
            <RetryButton requestId={requestId} retry={actions.retry} />
          )}
        </>
      }
    />
  )
}

import { Button } from '@/components/ui/button'
import { discardPrompt, retryPrompt } from '@/features/agent/stores/chat-outbox'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { describeSendFailure } from '@/features/agent/trays/lib/send-failure'
import { FailureNotice } from '@/features/agent/transcript/rows/failure-notice'

/** A message that did not go out, above the input box with its two ways on. */
export function SendFailureTray({
  chatId,
  item,
}: {
  chatId: string
  item: OutboxItem | undefined
}) {
  if (!item) return null
  const { headline, hint, final } = describeSendFailure(item)
  return (
    <FailureNotice
      kind="other"
      headline={headline}
      hint={hint}
      detail={item.error}
      headlineTestId="agent-send-failure"
      actions={
        <>
          <Button size="sm" variant="outline" onClick={() => discardPrompt(chatId, item.requestId)}>
            Discard
          </Button>
          {!final && (
            <Button
              size="sm"
              onClick={() => retryPrompt(item.wsId, chatId, item.requestId, item.steer)}
            >
              Retry
            </Button>
          )}
        </>
      }
    />
  )
}

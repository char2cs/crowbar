import { Button } from '@/components/ui/button'
import { discardPrompt, retryPrompt } from '@/features/agent/stores/chat-outbox'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { describeSendFailure } from '@/features/agent/trays/lib/send-failure'
import { FailureNotice } from '@/features/agent/transcript/rows/failure-notice'

/** A message that did not go out, above the input box with its two ways on. */
export function SendFailureTray({
  chatId,
  item,
  onEdit,
}: {
  chatId: string
  item: OutboxItem | undefined
  /** Puts the failed message's text in the composer. */
  onEdit: (text: string) => void
}) {
  if (!item) return null
  const { headline, hint, final } = describeSendFailure(item)
  return (
    <FailureNotice
      tone="danger"
      headline={headline}
      hint={hint}
      headlineTestId="agent-send-failure"
      actions={
        <>
          <Button
            size="sm"
            variant="outline"
            className="bg-background"
            onClick={() => {
              discardPrompt(chatId, item.requestId)
              onEdit(item.text)
            }}
          >
            Edit
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="bg-background"
            onClick={() => discardPrompt(chatId, item.requestId)}
          >
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

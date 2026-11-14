import { memo, useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useLiveText } from '@/features/agent/hooks/use-chat-feed'
import { turnTimestampLabel, turnTimeTitle } from '@/features/agent/lib/turn-time'
import { CopyIcon } from '@/features/agent/shared/agent-icons'
import { fuller } from '@/features/agent/stores/lib/fold-live'
import type { TextItem } from '@/features/agent/stores/lib/transcript-items'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { MessageBoundary } from '@/features/agent/transcript/rows/message-boundary'
import { toast } from '@/features/window/stores/toast-store'

async function copyTurn(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied to clipboard')
  } catch {
    toast.error('Could not copy to clipboard')
  }
}

/** The live renderer: follows the stream while the reply grows, then its row text. */
function LiveText({ chatId, item }: { chatId: string; item: TextItem }) {
  const live = useLiveText(chatId, item.messageId, 'answer')
  const text = item.settled ? item.text : fuller(item.text, live ?? '')
  return (
    <MarkdownMessage settled={item.settled} className="break-words">
      {text}
    </MarkdownMessage>
  )
}

function TextRowBody({
  chatId,
  item,
  effort,
  model,
  firstReply,
  turnbar,
}: {
  chatId: string
  item: TextItem
  effort?: string
  /** The model the turn ran on, where its CLI named one. */
  model?: string
  firstReply: boolean
  /** Only the reply that hands control back gets copy and time. */
  turnbar: boolean
}) {
  // R11: the renderer is picked once; settling changes the data, never the component.
  const [live] = useState(() => !item.settled)
  return (
    <article
      className="row"
      data-row-id={item.id}
      data-role="assistant"
      data-phase={item.phase || undefined}
      data-first-reply={firstReply ? 'true' : undefined}
    >
      <div className={cn('assistant', firstReply && 'frozen')}>
        <MessageBoundary text={item.text}>
          {live ? (
            <LiveText chatId={chatId} item={item} />
          ) : (
            <MarkdownMessageStatic diagrams variant="chat" className="break-words">
              {item.text}
            </MarkdownMessageStatic>
          )}
        </MessageBoundary>
        {item.settled && turnbar && (
          <div className="turnbar" data-testid="message-turn-actions">
            {model && (
              <span className="turn-effort" data-testid="message-model">
                {model}
              </span>
            )}
            {effort && (
              <span className="turn-effort" data-testid="message-effort">
                {effort}
              </span>
            )}
            <Button
              size="icon-sm"
              variant="ghost"
              tooltip="Copy"
              aria-label="Copy this message"
              onClick={() => void copyTurn(item.text)}
            >
              <CopyIcon size={12} className="size-3" />
            </Button>
            <time className="turn-time" dateTime={item.at} title={turnTimeTitle(item.at)}>
              {turnTimestampLabel(item.at)}
            </time>
          </div>
        )}
      </div>
    </article>
  )
}

export const TextRow = memo(TextRowBody)

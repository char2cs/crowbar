import { CloseIcon, PencilIcon, RetryIcon, TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { cn } from '@/lib/utils'

/**
 * A prompt waiting its turn.
 *
 * Dashed, because it is not in the record yet — the ledger holds only what the
 * provider has confirmed, and this row exists precisely for the gap between
 * pressing send and that confirmation arriving.
 *
 * It says almost nothing. Crowbar CANNOT know whether the agent has seen an
 * unconfirmed prompt, so the row does not narrate a state it is guessing at; it
 * shows the text, and the two or three things a person can actually do about it.
 * What it does say — retry, edit, cancel — is only offered where it works: an
 * uncertain delivery has no Edit, because editing would mint a new request for
 * text the provider may already have.
 */
export function QueuedRow({
  item,
  showTerminalHint,
  onEdit,
  onCancel,
  onRetry,
  onOpenTerminal,
}: {
  item: PromptQueueItem
  /** The delivery is old enough that the provider may be blocked on a prompt
   *  only its terminal can answer. A GUESS, and only offered where the daemon
   *  has no authoritative answer of its own. */
  showTerminalHint: boolean
  onEdit: () => void
  onCancel: () => void
  onRetry: () => void
  onOpenTerminal: () => void
}) {
  const uncertain = item.state === 'outcome_uncertain'
  const failed = item.state === 'failed' || uncertain
  const multi = item.text.length > 60 || item.text.includes('\n')

  return (
    <article
      className={cn('queued', multi && 'multi', failed && 'bad')}
      data-client-request-id={item.clientRequestId}
      data-state={item.state}
      data-testid="queued-prompt"
    >
      {/* THE PROMPT TEXT ALWAYS. An error is a second fact about this row, never
          a replacement for the thing the person actually wrote — losing their
          words to a failure message is how a retry becomes a retype. */}
      <span className="txt" title={item.text}>
        {item.text}
      </span>
      {item.error && <span className="err">{item.error}</span>}
      {showTerminalHint && (
        <Button
          size="icon-sm"
          variant="ghost"
          tooltip="The provider may be waiting for an answer in its terminal"
          aria-label="Open Terminal"
          onClick={onOpenTerminal}
        >
          <TerminalIcon />
        </Button>
      )}
      {(item.state === 'queued' || failed) && (
        <span className="acts">
          {failed && (
            <Button
              size="icon-sm"
              variant="ghost"
              tooltip={uncertain ? 'Retry with the same request ID' : 'Retry'}
              aria-label={uncertain ? 'Retry same request' : 'Retry prompt'}
              onClick={onRetry}
            >
              <RetryIcon />
            </Button>
          )}
          {!uncertain && (
            <Button
              size="icon-sm"
              variant="ghost"
              tooltip="Edit"
              aria-label="Edit queued prompt"
              onClick={onEdit}
            >
              <PencilIcon />
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="ghost"
            tooltip="Cancel"
            aria-label="Cancel queued prompt"
            onClick={onCancel}
          >
            <CloseIcon />
          </Button>
        </span>
      )}
    </article>
  )
}

import { MessageSquareIcon, SquareSplitHorizontalIcon, TerminalIcon } from 'lucide-react'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { cn } from '@/lib/utils'

interface ViewSwitcherProps {
  presentation: ChatPresentation
  /** The dev-only split instrument is on offer. */
  splitEnabled: boolean
  /**
   * The provider cannot hand a live turn to its native view.
   *
   * Undefined — the shipped case — means it can: both surfaces stay mounted and
   * point at the same process, so switching costs nothing. A provider that
   * declares otherwise disables the terminal side mid-turn rather than hiding
   * it, because the control still works the moment the turn ends.
   */
  handoverBlocked?: boolean
  onSelect: (next: ChatPresentation) => void
}

/**
 * Crowbar's chat, or the provider's own terminal.
 *
 * `ui/tabs`' pill variant, icon-only, sitting with the provider controls because
 * that is what it is: a statement about which face of this provider you are
 * looking at, not a window arrangement.
 *
 * Split is a DIAGNOSTIC and appears only where it is enabled — it puts the chat
 * Crowbar reconstructed next to the CLI's own output, which is how a turn that
 * never closed becomes visible by comparison instead of by bisection.
 */
export function ViewSwitcher({
  presentation,
  splitEnabled,
  handoverBlocked,
  onSelect,
}: ViewSwitcherProps) {
  return (
    <span className="viewswitch" role="tablist" aria-label="View">
      <button
        type="button"
        role="tab"
        className={cn(presentation === 'chat' && 'on')}
        aria-selected={presentation === 'chat'}
        aria-label="Chat"
        title="Crowbar chat"
        onClick={() => onSelect('chat')}
      >
        <MessageSquareIcon />
      </button>
      <button
        type="button"
        role="tab"
        className={cn(presentation === 'terminal' && 'on')}
        aria-selected={presentation === 'terminal'}
        disabled={handoverBlocked}
        aria-label="Terminal"
        title={
          handoverBlocked
            ? 'This provider cannot hand a live turn over — finish or stop it first'
            : 'The provider’s own view'
        }
        onClick={() => onSelect('terminal')}
      >
        <TerminalIcon />
      </button>
      {splitEnabled && (
        <button
          type="button"
          role="tab"
          className={cn(presentation === 'split' && 'on')}
          aria-selected={presentation === 'split'}
          aria-label="Split"
          title="Both, side by side"
          onClick={() => onSelect('split')}
        >
          <SquareSplitHorizontalIcon />
        </button>
      )}
    </span>
  )
}

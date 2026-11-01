import { ChatIcon, SplitIcon, TerminalIcon } from '@/features/agent/shared/agent-icons'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { cn } from '@/lib/utils'

interface ViewSwitcherProps {
  presentation: ChatPresentation
  splitEnabled: boolean
  /** The provider cannot hand a live turn over. Disables rather than hides, since
   *  the control works again the moment the turn ends. */
  handoverBlocked?: boolean
  onSelect: (next: ChatPresentation) => void
}

/** Split is a diagnostic: the reconstructed chat beside the CLI's own output. */
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
        disabled={handoverBlocked && presentation === 'terminal'}
        aria-label="Chat"
        title={
          handoverBlocked && presentation === 'terminal'
            ? 'The agent is working in its terminal — finish or stop it first'
            : 'Crowbar chat'
        }
        onClick={() => onSelect('chat')}
      >
        <ChatIcon />
      </button>
      <button
        type="button"
        role="tab"
        className={cn(presentation === 'terminal' && 'on')}
        aria-selected={presentation === 'terminal'}
        disabled={handoverBlocked && presentation !== 'terminal'}
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
          <SplitIcon />
        </button>
      )}
    </span>
  )
}

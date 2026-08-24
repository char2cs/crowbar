import { useRef, useState } from 'react'
import { Dropdown, type MenuItem } from '@/components/ui/dropdown'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { CheckIcon, UpDownIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

// Wide enough for a display name and its mark. Its own number, not the
// selection menus' — those list model ids, this lists product names. An inline
// STYLE, because Dropdown locks its measured content width on open and would
// overwrite a class-set one.
const MENU_WIDTH_PX = 190

export interface AgentProviderPickerProps {
  /** The agent running this chat, or undefined before any runner has been on it. */
  provider?: AgentProvider
  providers: AgentProvider[]
  /** A turn is in flight, or a switch is already running. */
  disabled?: boolean
  onSwitch: (providerId: string) => void
}

/**
 * WHICH AGENT is running this chat.
 *
 * Its own control, beside the model and effort chips rather than folded into
 * them. They answer questions of different kinds: a model is a setting this
 * chat carries, and the provider is WHOSE CLI is on the other end — switching it
 * restarts a process and resumes the conversation inside a different program.
 * Sharing one menu made a restart look like a dropdown selection.
 *
 * Absent when there is nothing to switch to. A machine with one agent installed
 * gets no picker rather than a menu of one, which is the same house rule the
 * model catalogue follows: a control that cannot do anything should not be
 * drawn, not drawn and disabled.
 */
export function AgentProviderPicker({
  provider,
  providers,
  disabled,
  onSwitch,
}: AgentProviderPickerProps) {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)

  // Offered = installed AND enabled. A provider whose CLI is not on PATH cannot
  // be spawned, and one the user switched off in Settings asked not to be.
  const offered = providers.filter((candidate) => candidate.connected && candidate.enabled)
  // Somewhere to GO, not simply more than one name: a chat sitting on an agent
  // the user has since switched off still has a move available, and a machine
  // with one agent installed has none.
  if (!offered.some((candidate) => candidate.id !== provider?.id)) return null

  const items: MenuItem[] = offered.map((candidate) => ({
    id: candidate.id,
    label: candidate.displayName,
    icon:
      candidate.id === provider?.id ? (
        <CheckIcon size={12} data-testid="provider-tick" />
      ) : (
        <ProviderIcon svg={candidate.icon} className="size-3" />
      ),
    disabled: candidate.id === provider?.id,
    onClick: () => onSwitch(candidate.id),
  }))

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        disabled={disabled}
        // Not dressed as destructive, because it is not: the CLI restarts INTO
        // the same conversation and resumes it.
        title="The agent running this chat. Switching restarts its CLI and resumes the conversation."
        aria-label={`Agent: ${provider?.displayName ?? 'none'}`}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={cn('chip max-w-40', provider && 'set')}
      >
        {provider && <ProviderIcon svg={provider.icon} className="size-3" />}
        <span className="truncate">{provider?.displayName ?? 'Agent'}</span>
        <UpDownIcon size={12} className="opacity-55" />
      </button>
      <Dropdown
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        anchorRef={anchorRef}
        anchorSide="top"
        anchorAlign="start"
        className="min-w-0"
        style={{ width: MENU_WIDTH_PX }}
        items={items}
      />
    </>
  )
}

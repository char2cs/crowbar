import { createPortal } from 'react-dom'
import { useChatHeaderSlotElement } from '@/features/agent/hooks/chat-header-slot'
import { ViewSwitch } from '@/features/agent/controls/view-switch'
import { IS_MOBILE } from '@/utils/platform'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface ChatViewSwitchProps {
  presentation: ChatPresentation
  splitEnabled: boolean
  handoverBlocked?: boolean
  /** The provider has a terminal to switch to. */
  hasTerminal: boolean
  onSelect: (next: ChatPresentation) => void
}

/** The chat/terminal switch for the chat surface, in the pane header like the terminal's own,
 *  so it never sits in the input box. Both surfaces stay mounted: only one draws it. */
export function ChatViewSwitch({
  presentation,
  splitEnabled,
  handoverBlocked,
  hasTerminal,
  onSelect,
}: ChatViewSwitchProps) {
  const headerSlot = useChatHeaderSlotElement()
  // The phone's switch lives in the dock footer.
  if (IS_MOBILE || presentation === 'terminal' || !hasTerminal) return null
  const switcher = (
    <ViewSwitch
      presentation={presentation}
      splitEnabled={splitEnabled}
      handoverBlocked={handoverBlocked}
      onSelect={onSelect}
    />
  )
  if (headerSlot) return createPortal(switcher, headerSlot)
  return <div className="absolute top-2 right-3 z-10">{switcher}</div>
}

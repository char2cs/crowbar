import { createPortal } from 'react-dom'
import { useChatHeaderSlotElement } from '@/features/agent/hooks/chat-header-slot'
import { ViewSwitch } from '@/features/agent/controls/view-switch'
import { IS_MOBILE } from '@/utils/platform'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface ChatViewSwitchProps {
  presentation: ChatPresentation
  splitEnabled: boolean
  handoverBlocked?: boolean
  hasTerminal: boolean
  onSelect: (next: ChatPresentation) => void
}

export function ChatViewSwitch({
  presentation,
  splitEnabled,
  handoverBlocked,
  hasTerminal,
  onSelect,
}: ChatViewSwitchProps) {
  const headerSlot = useChatHeaderSlotElement()
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

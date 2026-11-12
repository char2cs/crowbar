import { ChatCircleIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import { SegmentedControl, SegmentedControlItem } from '@/components/ui/segmented-control'
import { Tabs } from '@/components/ui/tabs'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface FooterViewSwitchProps {
  presentation: ChatPresentation
  /** The provider cannot hand a live turn over; the terminal segment waits for the turn to end. */
  handoverBlocked?: boolean
  onSelect: (next: ChatPresentation) => void
}

/** The phone's chat/terminal switch, centred in the dock footer. Arrow keys move between the
 *  segments; the raised indicator slides. */
export function FooterViewSwitch({
  presentation,
  handoverBlocked,
  onSelect,
}: FooterViewSwitchProps) {
  return (
    <Tabs
      value={presentation === 'terminal' ? 'terminal' : 'chat'}
      onValueChange={(next) => onSelect(next as ChatPresentation)}
      className="footer-viewswitch gap-0"
    >
      <SegmentedControl
        className="rounded-full"
        render={<TabsPrimitive.List aria-label="View" activateOnFocus />}
      >
        <SegmentedControlItem
          render={<TabsPrimitive.Tab value="chat" aria-label="Chat" />}
          className="h-7 w-10 rounded-full px-0 sm:h-7"
        >
          <ChatCircleIcon size={18} />
        </SegmentedControlItem>
        <SegmentedControlItem
          render={
            <TabsPrimitive.Tab value="terminal" aria-label="Terminal" disabled={handoverBlocked} />
          }
          className="h-7 w-10 rounded-full px-0 sm:h-7"
        >
          <TerminalWindowIcon size={18} />
        </SegmentedControlItem>
      </SegmentedControl>
    </Tabs>
  )
}

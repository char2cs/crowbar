import { ChatCircleIcon, ColumnsIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import { SegmentedControl, SegmentedControlItem } from '@/components/ui/segmented-control'
import { Tabs } from '@/components/ui/tabs'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface ViewSwitchProps {
  presentation: ChatPresentation
  splitEnabled?: boolean
  handoverBlocked?: boolean
  onSelect: (next: ChatPresentation) => void
}

const SEGMENT = 'h-7 w-10 rounded-full px-0 sm:h-7'

export function ViewSwitch({
  presentation,
  splitEnabled,
  handoverBlocked,
  onSelect,
}: ViewSwitchProps) {
  const blocked = (face: ChatPresentation) =>
    Boolean(handoverBlocked) && (face === 'terminal') !== (presentation === 'terminal')
  return (
    <Tabs
      value={presentation}
      onValueChange={(next) => onSelect(next as ChatPresentation)}
      className="viewswitch gap-0"
    >
      <SegmentedControl
        className="rounded-full"
        render={<TabsPrimitive.List aria-label="View" activateOnFocus />}
      >
        <SegmentedControlItem
          render={<TabsPrimitive.Tab value="chat" aria-label="Chat" disabled={blocked('chat')} />}
          className={SEGMENT}
        >
          <ChatCircleIcon size={18} />
        </SegmentedControlItem>
        <SegmentedControlItem
          render={
            <TabsPrimitive.Tab
              value="terminal"
              aria-label="Terminal"
              disabled={blocked('terminal')}
            />
          }
          className={SEGMENT}
        >
          <TerminalWindowIcon size={18} />
        </SegmentedControlItem>
        {splitEnabled && (
          <SegmentedControlItem
            render={<TabsPrimitive.Tab value="split" aria-label="Split" />}
            className={SEGMENT}
          >
            <ColumnsIcon size={18} />
          </SegmentedControlItem>
        )}
      </SegmentedControl>
    </Tabs>
  )
}

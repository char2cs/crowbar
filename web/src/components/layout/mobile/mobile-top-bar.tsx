import type { ReactNode } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { Button } from '@/components/ui/button'
import { hasNativeChrome } from '@/lib/native-chrome'
import { CHAT_TITLE_TEXT } from '@/components/ui/chat-title-text'
import { cn } from '@/lib/utils'
import { MobileChatTitle } from './mobile-chat-title'

// In the phone app the corner buttons are native glass, drawn over this bar.
const NATIVE_CHROME = hasNativeChrome()

interface CornerButton {
  label: string
  icon: ReactNode
  disabled?: boolean
  onClick: () => void
}

/** The phone's top bar: the focused chat's title leading, beside the chat-list button, and
 *  web corner buttons only where no native chrome draws them. */
export function MobileTopBar({ trailing }: { trailing?: CornerButton }) {
  // Clear of the 44pt circles (16 inset + 44 + 8), and of the context meter once it shows.
  const trailingInset = useFocusedChatReportsUsage() ? 148 : 68
  return (
    <header className="relative h-[calc(env(safe-area-inset-top)+44px)] shrink-0 pt-[env(safe-area-inset-top)]">
      <div className="min-w-0" style={{ paddingLeft: 68, paddingRight: trailingInset }}>
        <MobileChatTitle className={cn(CHAT_TITLE_TEXT, 'truncate text-[22px] leading-[44px]')} />
      </div>
      {trailing && (
        <div className="absolute right-2 bottom-0 flex h-11 items-center">
          <Corner {...trailing} />
        </div>
      )}
    </header>
  )
}

function useFocusedChatReportsUsage(): boolean {
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? '')
  const wsId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.workspaceId ?? '')
  return useWorkspaceStoreById(
    wsId,
    (s) => s.agentChats.telemetry[chatId]?.context?.usedPercent !== undefined,
  )
}

function Corner({ label, icon, disabled, onClick }: CornerButton) {
  if (NATIVE_CHROME) return null
  return (
    <Button variant="ghost" size="icon" aria-label={label} disabled={disabled} onClick={onClick}>
      {icon}
    </Button>
  )
}

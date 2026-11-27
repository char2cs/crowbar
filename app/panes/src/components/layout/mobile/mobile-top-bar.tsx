import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { hasNativeChrome } from '@/lib/native-chrome'
import { CHAT_TITLE_TEXT } from '@/components/ui/chat-title-text'
import { cn } from '@/lib/utils'
import { MobileChatTitle } from './mobile-chat-title'

const NATIVE_CHROME = hasNativeChrome()

interface CornerButton {
  label: string
  icon: ReactNode
  disabled?: boolean
  onClick: () => void
}

export function MobileTopBar({ trailing }: { trailing?: CornerButton }) {
  return (
    <header className="relative h-[calc(env(safe-area-inset-top)+44px)] shrink-0 pt-[env(safe-area-inset-top)]">
      <div className="min-w-0" style={{ paddingLeft: 68, paddingRight: 68 }}>
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

function Corner({ label, icon, disabled, onClick }: CornerButton) {
  if (NATIVE_CHROME) return null
  return (
    <Button variant="ghost" size="icon" aria-label={label} disabled={disabled} onClick={onClick}>
      {icon}
    </Button>
  )
}

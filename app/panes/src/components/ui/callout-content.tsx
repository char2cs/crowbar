import * as React from 'react'

import { cn } from '@/lib/utils'

export function calloutClassName(className?: string) {
  return cn('my-1 flex rounded-sm bg-muted p-4 pl-3', className)
}

const EMOJI_FONT_FAMILY =
  '"Apple Color Emoji", "Segoe UI Emoji", NotoColorEmoji, "Noto Color Emoji", "Segoe UI Symbol", "Android Emoji", EmojiSymbols'

export function CalloutBody({
  icon,
  iconSlot,
  children,
}: {
  icon?: string
  iconSlot?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="flex w-full gap-2 rounded-md">
      {iconSlot ?? (
        <span
          className="flex size-6 select-none items-center justify-center p-1 text-[18px]"
          style={{ fontFamily: EMOJI_FONT_FAMILY }}
        >
          {icon || '💡'}
        </span>
      )}
      <div className="w-full">{children}</div>
    </div>
  )
}

export { EMOJI_FONT_FAMILY }

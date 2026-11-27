import { ChatsCircle } from '@phosphor-icons/react'
import { ProviderIcon } from './provider-icon'
import { cn } from '@/utils/cn'

interface ChatGlyphProps {
  svg: string
  className?: string
}

export function ChatGlyph({ svg, className }: ChatGlyphProps) {
  if (svg) {
    return <ProviderIcon svg={svg} className={cn('size-4', className)} />
  }
  return (
    <ChatsCircle
      data-chat-glyph="true"
      aria-hidden="true"
      className={cn('size-4 shrink-0', className)}
      weight="regular"
    />
  )
}

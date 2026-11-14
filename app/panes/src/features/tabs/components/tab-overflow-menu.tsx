import { CaretDownIcon } from '@phosphor-icons/react'
import { type RefObject, useEffect, useLayoutEffect, useState } from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

interface TabOverflowMenuProps {
  /** The strip's horizontally scrolling element; its own size decides whether tabs fit. */
  scrollRef: RefObject<HTMLDivElement | null>
  tabs: readonly { id: string; name: string }[]
  activeId: string | null
  onSelect: (id: string) => void
}

function overflows(el: HTMLElement | null): boolean {
  return !!el && el.scrollWidth > el.clientWidth + 1
}

/** "More tabs": every open tab in one list, drawn only while the strip cannot show them all. */
export function TabOverflowMenu({ scrollRef, tabs, activeId, onSelect }: TabOverflowMenuProps) {
  const [hidden, setHidden] = useState(false)

  // The strip's layout is the owner of "does it fit": re-read it when the tabs change.
  useLayoutEffect(() => {
    setHidden(overflows(scrollRef.current))
  }, [scrollRef, tabs])

  // ...and when the strip itself is resized (window, split, sidebar).
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const observer = new ResizeObserver(() => setHidden(overflows(el)))
    observer.observe(el)
    return () => observer.disconnect()
  }, [scrollRef])

  if (!hidden) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="More tabs"
        title="More tabs"
        className="inline-flex size-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground outline-none hover:bg-sidebar-element-hover focus-visible:ring-2 focus-visible:ring-ring"
      >
        <CaretDownIcon className="size-3.5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-auto max-w-80">
        {tabs.map((tab) => (
          <DropdownMenuItem
            key={tab.id}
            onClick={() => onSelect(tab.id)}
            className={cn('truncate', tab.id === activeId && 'font-semibold')}
          >
            {tab.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

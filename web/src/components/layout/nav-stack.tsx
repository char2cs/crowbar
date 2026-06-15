import { type ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

interface NavStackProps {
  children: ReactNode
}

export function NavStack({ children }: NavStackProps) {
  const stack = useSidebarNavStore((s) => s.stack)
  const pop = useSidebarNavStore((s) => s.pop)
  const hasScreen = stack.length > 0

  return (
    <div className="relative flex flex-1 flex-col overflow-hidden">
      {/* Root layer — always mounted, pushed back when a screen is active */}
      <div
        className={cn(
          'absolute inset-0 flex flex-col transition-[transform,opacity] duration-[280ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
          hasScreen ? '-translate-x-1/4 opacity-40 pointer-events-none' : 'translate-x-0 opacity-100',
        )}
      >
        {children}
      </div>

      {/* Pushed screens */}
      {stack.map((screen, i) => {
        const isTop = i === stack.length - 1
        return (
          <div
            key={screen.id}
            className={cn(
              'absolute inset-0 flex flex-col transition-[transform,opacity] duration-[280ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
              isTop ? 'translate-x-0 opacity-100' : '-translate-x-1/4 opacity-0 pointer-events-none',
              !isTop && 'translate-x-full',
            )}
          >
            {/* Header with back button */}
            <div className="flex items-center gap-2 border-b border-border px-2.5 py-2 flex-shrink-0">
              <button
                aria-label="Back"
                onClick={pop}
                className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-md bg-accent/50 text-muted-foreground hover:bg-accent hover:text-foreground"
              >
                <ChevronLeft className="size-3.5" />
              </button>
              <span className="truncate font-mono text-[11px] text-foreground">
                {screen.title}
              </span>
            </div>
            <div className="flex flex-1 flex-col overflow-hidden">
              {screen.component}
            </div>
          </div>
        )
      })}
    </div>
  )
}

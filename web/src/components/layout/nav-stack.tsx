import { type ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC } from '@/utils/platform'

interface NavStackProps {
  children: ReactNode
}

export function NavStack({ children }: NavStackProps) {
  const stack = useSidebarNavStore((s) => s.stack)
  const pop = useSidebarNavStore((s) => s.pop)
  const hasScreen = stack.length > 0
  const isRight = useSettingsStore((s) => s.settings.sidebarPosition === 'right')

  return (
    <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
      {/* Root layer — normal flow so pointer capture works during workspace drag */}
      <div
        className={cn(
          'flex h-full flex-col transition-[transform,opacity] duration-[280ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
          hasScreen && '-translate-x-1/4 opacity-0 pointer-events-none',
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
              isTop
                ? 'translate-x-0 opacity-100'
                : '-translate-x-1/4 opacity-0 pointer-events-none',
              !isTop && 'translate-x-full',
            )}
          >
            {/* Header — same height and padding as SidebarProjectHeader */}
            <div
              data-tauri-drag-region
              className={cn(
                'flex w-full flex-shrink-0 items-center gap-2 border-b border-border px-3',
                IS_MAC ? 'h-[44px]' : 'h-[34px]',
              )}
            >
              {IS_MAC && !isRight && <div className="w-[72px] shrink-0" />}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Back"
                onClick={pop}
                className="shrink-0"
              >
                <ChevronLeft />
              </Button>
              <span className="min-w-0 flex-1 truncate text-[13px] font-semibold text-foreground">
                {screen.title}
              </span>
            </div>

            <div className="flex flex-1 flex-col overflow-hidden">{screen.component}</div>
          </div>
        )
      })}
    </div>
  )
}

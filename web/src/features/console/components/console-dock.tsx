import { useEffect, type ReactNode } from 'react'
import { useConsoleKeyboard } from '@/features/keymaps/hooks/use-console-keyboard'
import { createConsoleController } from '@/features/console/lib/controller'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { createTransport } from '@/lib/ws/manager'
import { isWebSocketCapable } from '@/lib/ws/url'
import { cn } from '@/utils/cn'
import { ConsolePanel } from './console-panel'

const toggleConsole = () => useConsoleStore.getState().toggle()

/**
 * Wraps the whole app shell so the console can cover the sidebar too (overlay) or
 * share the window with it (push), and ties the log stream to it being open.
 */
export function ConsoleDock({ children }: { children: ReactNode }) {
  const vertical = useConsoleStore((s) => s.dock === 'top' || s.dock === 'bottom')
  useConsoleKeyboard(toggleConsole)

  useEffect(() => {
    if (!isWebSocketCapable()) return
    const controller = createConsoleController({
      store: useConsoleStore,
      open: createTransport,
      timers: {
        set: (fn, ms) => setTimeout(fn, ms),
        clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
      },
    })
    return controller.dispose
  }, [])

  return (
    <div
      className={cn('relative flex h-screen w-full overflow-hidden', vertical && 'flex-col')}
      data-slot="console-dock"
    >
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
      <ConsolePanel />
    </div>
  )
}

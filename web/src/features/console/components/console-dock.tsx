import { useEffect } from 'react'
import { useConsoleKeyboard } from '@/features/keymaps/hooks/use-console-keyboard'
import { createConsoleController } from '@/features/console/lib/controller'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { createTransport } from '@/lib/ws/manager'
import { isWebSocketCapable } from '@/lib/ws/url'
import { ConsolePanel } from './console-panel'

const toggleConsole = () => useConsoleStore.getState().toggle()

/** Hosts the console in the content column and ties the log stream to it being open. */
export function ConsoleDock() {
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

  return <ConsolePanel />
}

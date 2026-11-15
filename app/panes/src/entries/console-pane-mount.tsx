import { StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { createConsoleController, type ConsoleController } from '@/features/console/lib/controller'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { createTransport } from '@/lib/ws/manager'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { ConsolePane } from './console-pane'

/** Shows the daemon log while the pane is mounted: the log stream is open exactly then. */
export class ConsolePaneMount {
  private root: Root | null = null
  private controller: ConsoleController | null = null

  constructor(
    private readonly container: HTMLElement,
    private readonly client: PaneClient,
  ) {}

  /** Starts following the shell's lifecycle messages. */
  start(): void {
    this.client.onLifecycle(({ event }: LifecycleMessage) => {
      if (event === 'mount') this.mount()
      if (event === 'park' || event === 'destroy') this.unmount()
    })
  }

  private mount(): void {
    useConsoleStore.getState().setOpen(true)
    this.controller ??= createConsoleController({
      store: useConsoleStore,
      open: createTransport,
      timers: {
        set: (fn, ms) => setTimeout(fn, ms),
        clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
      },
    })
    this.root ??= createRoot(this.container)
    this.root.render(
      <StrictMode>
        <ConsolePane />
      </StrictMode>,
    )
  }

  private unmount(): void {
    this.controller?.dispose()
    this.controller = null
    useConsoleStore.getState().setOpen(false)
    this.root?.unmount()
    this.root = null
  }
}

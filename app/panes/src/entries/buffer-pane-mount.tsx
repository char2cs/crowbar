import { StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MotionConfig } from 'framer-motion'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { BufferPaneHost } from './buffer-pane-host'
import { bufferSession } from './buffer-session'
import type { OpenBuffer } from './open-buffer'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'

/** Mounts and unmounts a buffer page's React tree (a file or a review in the pane's one tab) as the shell's lifecycle messages arrive. */
export class BufferPaneMount {
  private root: Root | null = null

  constructor(
    private readonly container: HTMLElement,
    private readonly client: PaneClient,
    private readonly open: OpenBuffer,
  ) {}

  /** Starts following the shell's lifecycle messages. */
  start(): void {
    this.client.onLifecycle((message) => this.handle(message))
  }

  private handle({ event, params, state }: LifecycleMessage): void {
    if (event === 'mount') this.mount(params.threadId, state)
    if (event === 'activate') bufferSession.setState({ active: true })
    if (event === 'deactivate') bufferSession.setState({ active: false })
    if (event === 'park' || event === 'destroy') this.unmount()
  }

  private mount(chatId: string | undefined, saved: unknown): void {
    if (!chatId) throw new Error('the pane was opened without a threadId')
    const { panes } = windowPaneStore.getState()
    windowPaneStore.setState({ panes: { ...panes, [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null) } })
    bufferSession.setState({ chatId, saved, active: false })
    this.root ??= createRoot(this.container)
    this.root.render(
      <StrictMode>
        <FontStyleInjector />
        <MotionConfig reducedMotion="user">
          <TooltipProvider>
            <PrimitiveDialogProvider>
              <BufferPaneHost client={this.client} open={this.open} />
            </PrimitiveDialogProvider>
          </TooltipProvider>
        </MotionConfig>
      </StrictMode>,
    )
  }

  private unmount(): void {
    this.root?.unmount()
    this.root = null
    bufferSession.setState({ chatId: null, saved: null, active: false })
  }
}

import { StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MotionConfig } from 'framer-motion'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { initialViewState } from '@/features/panes/lib/view-state'
import { fillPane } from '@/features/panes/lib/view-ops'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { ChatPaneHost } from './chat-pane-host'
import { chatSession } from './chat-session'

/** Mounts and unmounts the chat page's React tree as the shell's lifecycle messages arrive. */
export class ChatPaneMount {
  private root: Root | null = null

  constructor(
    private readonly container: HTMLElement,
    private readonly client: PaneClient,
  ) {}

  /** Starts following the shell's lifecycle messages. */
  start(): void {
    this.client.onLifecycle((message) => this.handle(message))
  }

  private handle({ event, params }: LifecycleMessage): void {
    if (event === 'mount') this.mount(params.threadId)
    if (event === 'activate') chatSession.setState({ active: true })
    if (event === 'deactivate') chatSession.setState({ active: false })
    if (event === 'park' || event === 'destroy') this.unmount()
  }

  private mount(chatId: string | undefined): void {
    if (!chatId) throw new Error('the chat pane was opened without a threadId')
    windowPaneStore.setState((state) => {
      Object.assign(state, initialViewState())
      fillPane(state, ROOT_PANE_ID, chatId, null, '')
      return state
    })
    chatSession.setState({ chatId, active: false })
    this.root ??= createRoot(this.container)
    this.root.render(
      <StrictMode>
        <MotionConfig reducedMotion="user">
          <TooltipProvider>
            <PrimitiveDialogProvider>
              <ChatPaneHost client={this.client} />
            </PrimitiveDialogProvider>
          </TooltipProvider>
        </MotionConfig>
      </StrictMode>,
    )
  }

  private unmount(): void {
    this.root?.unmount()
    this.root = null
    chatSession.setState({ chatId: null, active: false })
  }
}

import { StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MotionConfig } from 'framer-motion'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { initialViewState, type ViewState } from '@/features/panes/lib/view-state'
import { fillPane } from '@/features/panes/lib/view-ops'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import type { PaneParams } from '@/sdk/pane-params'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { ChatPaneHost } from './chat-pane-host'
import { chatSession } from './chat-session'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'

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
    this.client.onGroup((fillsGroup) => chatSession.setState({ fillsGroup }))
    windowPaneStore.subscribe((state) => this.follow(state.panes[ROOT_PANE_ID]?.chatId ?? null))
  }

  private follow(chatId: string | null): void {
    const shown = chatSession.getState().chatId
    const repointed = !!chatId && !!shown && chatId !== shown
    if (!repointed) return
    chatSession.setState({ chatId })
    this.client.setParams({ threadId: chatId })
  }

  private handle({ event, params }: LifecycleMessage): void {
    if (event === 'mount') this.mount(params)
    if (event === 'activate') chatSession.setState({ active: true })
    if (event === 'deactivate') chatSession.setState({ active: false })
    if (event === 'park' || event === 'destroy') this.unmount()
  }

  private mount(params: PaneParams): void {
    const chatId = params.threadId
    if (!chatId) throw new Error('the chat pane was opened without a threadId')
    chatSession.setState({ chatId, active: false })
    windowPaneStore.setState((state) => {
      Object.assign(state, initialViewState())
      fillPane(state, ROOT_PANE_ID, chatId, null, '')
      placeDraft(state, params)
      return state
    })
    this.root ??= createRoot(this.container)
    this.root.render(
      <StrictMode>
        <FontStyleInjector />
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
    chatSession.setState({ chatId: null, active: false, fillsGroup: false })
  }
}

function placeDraft(state: ViewState, params: PaneParams): void {
  const root = state.panes[ROOT_PANE_ID]
  if (!root) return
  root.workspaceId = params.workspaceId ?? null
  if (params.parentId) root.draftParentId = params.parentId
  const view = root.viewId ? state.views[root.viewId] : undefined
  const { projectId, workspaceId: wsId, repoId = '' } = params
  const placed = view && projectId && wsId
  if (!placed) return
  view.projectId = projectId
  recordWorkspaceScope({ projectId, repoId, wsId })
}

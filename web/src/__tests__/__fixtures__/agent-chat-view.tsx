import type { ComponentProps } from 'react'
import { render } from '@testing-library/react'
import { PlateController, useEditorRef, type PlateEditor } from 'platejs/react'
import { AgentChatView } from '@/features/agent/chat/agent-chat-view'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

export const grabbed: { editor?: PlateEditor } = {}
function GrabEditor() {
  grabbed.editor = useEditorRef()
  return null
}

export function drawChatView(
  chatId: string,
  overrides: Partial<ComponentProps<typeof AgentChatView>> = {},
) {
  const props: ComponentProps<typeof AgentChatView> = {
    wsId: 'w1',
    chatId,
    providerId: 'provider-b',
    providers: [],
    live: true,
    canSend: true,
    active: true,
    visible: true,
    isActivePane: true,
    onOpenTerminal: () => {},
    provider: 'provider-b',
    model: '',
    effort: '',
    onSelectionChange: () => {},
    presentation: 'chat',
    splitEnabled: false,
    onSelectPresentation: () => {},
    ...overrides,
  }
  // An empty workspace store: no list in any store names this chat.
  return render(
    <PlateController>
      <WorkspaceStoreContext.Provider value={createWorkspaceStore('w1')}>
        <AgentChatView {...props} />
      </WorkspaceStoreContext.Provider>
      <GrabEditor />
    </PlateController>,
  )
}

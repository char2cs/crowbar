import { sendPrompt } from '@/features/agent/stores/chat-outbox'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'

/** Where a draft thread lives, and where its chat will be created. */
export interface DraftSpec {
  paneId: string
  draftId: string
  projectId: string
  workspaceId: string
  /** The row the chat is placed under; '' is the panel root. */
  parentId: string
}

/** The first prompt, with the selection it was written under. */
export interface DraftPrompt {
  text: string
  provider: string
  model: string
  effort: string
}

/** The draft a pane holds, or null when it holds none (or has been closed). */
export function draftOfPane(paneId: string): DraftSpec | null {
  const { panes, views } = windowPaneStore.getState()
  const pane = panes[paneId]
  if (!pane?.chatId || !pane.workspaceId || !pane.viewId) return null
  const projectId = views[pane.viewId]?.projectId
  if (!projectId) return null
  return {
    paneId,
    draftId: pane.chatId,
    projectId,
    workspaceId: pane.workspaceId,
    parentId: pane.draftParentId ?? '',
  }
}

/** Where a draft's chat starts: on Chat, which sends the prompt (Terminal holds it unsent),
 *  and on the agent the prompt was written for. */
export function draftLanding(prompt: DraftPrompt): {
  presentation: LandingChatPresentation
  providerId: string
} {
  return { presentation: 'chat', providerId: prompt.provider }
}

/** The draft became a chat: its first prompt goes to that chat's outbox, which
 *  sends it once the chat can take it, and the pane re-points at it in the same step. */
export function finishDraftThread(draft: DraftSpec, chatId: string, prompt: DraftPrompt): void {
  sendPrompt(draft.workspaceId, chatId, { ...prompt, steer: false })
  const { panes, paneActions } = windowPaneStore.getState()
  if (panes[draft.paneId]?.chatId !== draft.draftId) return
  paneActions.retargetPane(draft.paneId, chatId, null)
}

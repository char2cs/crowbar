import { newQueuedPrompt, savePromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

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

/**
 * The draft became a chat: its first prompt is queued for that chat, and the
 * pane re-points at it in the same step. The queue is written first so the
 * chat's view finds the prompt on its first render and sends it.
 */
export function finishDraftThread(draft: DraftSpec, chatId: string, prompt: DraftPrompt): void {
  savePromptQueue(draft.workspaceId, chatId, [
    newQueuedPrompt(prompt.text, 0, prompt.provider, prompt.model, prompt.effort),
  ])
  const { panes, paneActions } = windowPaneStore.getState()
  if (panes[draft.paneId]?.chatId !== draft.draftId) return
  paneActions.retargetPane(draft.paneId, chatId, null)
}

import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { draftLanding, draftOfPane, finishDraftThread, type DraftPrompt } from './draft-thread'
import { startThread } from './thread-create'

// Turns the pane's draft into a real chat with `prompt` as its first message. A refused create
// toasts and leaves the draft to send again. `typedSince` reads the draft's box as the chat opens.
// Kept out of the sidebar's create actions so the chat outbox stays out of the boot bundle.
export function promoteDraftThread(
  paneId: string,
  prompt: DraftPrompt,
  typedSince: () => string,
): Promise<void> {
  const draft = draftOfPane(paneId)
  if (!draft) return Promise.resolve()
  const home = getHomeWorkspaceId(draft.projectId) === draft.workspaceId
  return startThread({
    inFlightKey: `draft:${draft.draftId}`,
    projectId: draft.projectId,
    workspaceId: draft.workspaceId,
    // A home draft lands at the panel root; a repo draft under its row, else its workspace.
    parentId: home ? '' : draft.parentId || draft.workspaceId,
    ...draftLanding(prompt),
    onCreated: (chatId) => finishDraftThread(draft, chatId, prompt, typedSince),
  })
}

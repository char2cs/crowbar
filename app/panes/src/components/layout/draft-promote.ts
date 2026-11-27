import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { draftLanding, draftOfPane, finishDraftThread, type DraftPrompt } from './draft-thread'
import { startThread } from './thread-create'

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
    parentId: home ? '' : draft.parentId || draft.workspaceId,
    ...draftLanding(prompt),
    onCreated: (chatId) => finishDraftThread(draft, chatId, prompt, typedSince),
  })
}

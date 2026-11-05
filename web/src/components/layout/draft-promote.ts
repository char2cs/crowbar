import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { promoteRepoDraft } from './create-actions'
import { draftOfPane, type DraftPrompt } from './draft-thread'
import { promoteHomeDraft } from './home-actions'

// Turns the pane's draft into a real chat with `prompt` as its first message. A refused create
// toasts and leaves the draft to send again.
export function promoteDraftThread(paneId: string, prompt: DraftPrompt): Promise<void> {
  const draft = draftOfPane(paneId)
  if (!draft) return Promise.resolve()
  return getHomeWorkspaceId(draft.projectId) === draft.workspaceId
    ? promoteHomeDraft(draft, prompt)
    : promoteRepoDraft(draft, prompt)
}

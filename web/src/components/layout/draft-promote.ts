import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { promoteRepoDraft } from './create-actions'
import { draftOfPane, type DraftPrompt } from './draft-thread'
import { promoteHomeDraft } from './home-actions'

// Turns the pane's draft into a real chat with `prompt` as its first message. A refused create
// toasts and leaves the draft to send again. `typedSince` reads the draft's box as the chat opens.
export function promoteDraftThread(
  paneId: string,
  prompt: DraftPrompt,
  typedSince: () => string,
): Promise<void> {
  const draft = draftOfPane(paneId)
  if (!draft) return Promise.resolve()
  return getHomeWorkspaceId(draft.projectId) === draft.workspaceId
    ? promoteHomeDraft(draft, prompt, typedSince)
    : promoteRepoDraft(draft, prompt, typedSince)
}

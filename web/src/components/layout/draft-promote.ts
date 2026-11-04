import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { promoteRepoDraft } from './create-actions'
import { draftOfPane, type DraftPrompt } from './draft-thread'
import { promoteHomeDraft } from './home-actions'

/**
 * The draft in `paneId` becomes a real chat carrying `prompt` as its first
 * message. Resolves when the create settles; a refused create toasts and
 * leaves the draft in place to be sent again.
 */
export function promoteDraftThread(paneId: string, prompt: DraftPrompt): Promise<void> {
  const draft = draftOfPane(paneId)
  if (!draft) return Promise.resolve()
  return getHomeWorkspaceId(draft.projectId) === draft.workspaceId
    ? promoteHomeDraft(draft, prompt)
    : promoteRepoDraft(draft, prompt)
}

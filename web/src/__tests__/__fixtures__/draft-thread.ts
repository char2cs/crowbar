import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import type { DraftPrompt } from '@/components/layout/draft-thread'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export const FIRST_PROMPT: DraftPrompt = { text: 'hello', provider: '', model: '', effort: '' }

/** The pane currently holding a draft thread, if any. */
export function draftPaneId(): string | undefined {
  return Object.values(windowPaneStore.getState().panes).find((p) => isDraftChatId(p.chatId))?.id
}

/**
 * "+ thread" as a person uses it: the click, then — when it opened a blank
 * draft rather than creating a chat outright — the first send.
 */
export async function createThread(
  parentId: string,
  ...rest: [navigate: Parameters<typeof handleCreate>[2], presentation?: 'chat' | 'terminal']
): Promise<void> {
  await handleCreate(parentId, 'thread', ...rest)
  const paneId = draftPaneId()
  if (paneId) await promoteDraftThread(paneId, FIRST_PROMPT)
}

/** The project-home "New thread", then the first send from its draft. */
export async function createHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: Parameters<typeof handleCreateHomeThread>[2],
): Promise<void> {
  windowPaneStore.setState({ activeProjectId: projectId })
  await handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
  const paneId = draftPaneId()
  if (paneId) await promoteDraftThread(paneId, FIRST_PROMPT)
}

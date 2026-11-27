import { afterEach } from 'vitest'
import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import type { DraftPrompt } from '@/components/layout/draft-thread'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import { forgetOutbox } from '@/features/agent/stores/chat-outbox'
import { chatStore } from '@/features/agent/stores/chat-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export const FIRST_PROMPT: DraftPrompt = { text: 'hello', provider: '', model: '', effort: '' }

// The outbox outlives a test, and would send these prompts after the environment is gone.
afterEach(() => {
  for (const chatId of Object.keys(chatStore.getState().outbox)) forgetOutbox(chatId)
})

export function draftPaneId(): string | undefined {
  return Object.values(windowPaneStore.getState().panes).find((p) => isDraftChatId(p.chatId))?.id
}

export async function createThread(
  parentId: string,
  ...rest: [navigate: Parameters<typeof handleCreate>[2], presentation?: 'chat' | 'terminal']
): Promise<void> {
  await handleCreate(parentId, 'thread', ...rest)
  const paneId = draftPaneId()
  if (paneId) await promoteDraftThread(paneId, FIRST_PROMPT, () => '')
}

export async function createHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: Parameters<typeof handleCreateHomeThread>[2],
): Promise<void> {
  windowPaneStore.setState({ activeProjectId: projectId })
  await handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
  const paneId = draftPaneId()
  if (paneId) await promoteDraftThread(paneId, FIRST_PROMPT, () => '')
}

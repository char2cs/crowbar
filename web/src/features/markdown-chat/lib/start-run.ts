import { postRun, startRun } from '@/lib/api/run'
import { useSidebarStore } from '@/lib/store/sidebar'

/**
 * Kicks off an agent run for a chat: resolves the chat's workspace from the
 * sidebar store, creates the run (POST /v0/workspaces/:wsId/runs) and starts
 * it (POST /v0/runs/:id/start). Resolves to the run id; rejects when the chat
 * is unknown or either request fails — callers must surface that as an error
 * state on the pending assistant turn.
 */
export async function startAgentRunForChat(chatId: string): Promise<string> {
  const wsId = useSidebarStore.getState().chats.find((c) => c.id === chatId)?.wsId
  if (!wsId) {
    throw new Error('this conversation is not linked to a backend chat')
  }
  const run = await postRun(wsId, chatId)
  await startRun(run.id)
  return run.id
}

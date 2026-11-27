import type { PaneGroup } from '@/features/panes/types/pane'
import { paneChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace-id'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { requestWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'

export type ReadPanes = () => Record<string, PaneGroup>

function chatIsUp(readPanes: ReadPanes, chatId: string): boolean {
  return Object.values(readPanes()).some((pane) => pane.chatId === chatId)
}

export function releaseClosedChat(
  chatId: string,
  workspaceId: string | null,
  readPanes: ReadPanes,
): void {
  if (chatIsUp(readPanes, chatId)) return
  if (!workspaceId) return

  for (const pane of Object.values(readPanes())) {
    if (paneChatWorkspaceId(pane) === workspaceId) return
  }

  if (workspaceId === getActiveWorkspaceId()) return

  requestWorkspaceEviction(workspaceId)
}

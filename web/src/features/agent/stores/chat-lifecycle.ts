import { entityStore, type EntityState } from '@/core/entities/entity-store'
import type { ChatEntity } from '@/core/entities/chat-entity'
import { clearExcalidrawEditRequest } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { forgetOutbox } from '@/features/agent/stores/chat-outbox'
import { dropFeed } from '@/features/agent/stores/chat-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { forgetWorkspaceScope, recordWorkspaceScope } from '@/lib/workspace-scope'

type Chats = EntityState['byKind']['chat']
type Workspaces = EntityState['byKind']['workspace']

/** A chat the daemon deleted: everything this client keeps for it goes with it. */
function forget(chatId: string) {
  dropFeed(chatId)
  forgetOutbox(chatId)
  clearScrollPosition(chatId)
  clearExcalidrawEditRequest(chatId)
  windowPaneStore.getState().paneActions.forgetChat(chatId)
}

function releaseRunner(runnerId: string, chatId: string) {
  const { panes, paneActions } = windowPaneStore.getState()
  for (const pane of Object.values(panes)) {
    if (pane.runnerId === runnerId && pane.chatId === chatId) {
      paneActions.setPaneRunner(pane.id, null)
    }
  }
}

/** A chat that started working with no pane showing it runs in the background, findable. */
function adoptIfViewless(chat: ChatEntity) {
  const { panes, paneActions } = windowPaneStore.getState()
  if (chatPaneIndex(panes).has(chat.id) || !chat.projectId) return
  paneActions.adoptBackgroundChat(chat.id, chat.projectId, chat.workspaceId)
}

function followChats(next: Chats, prev: Chats, live: boolean) {
  for (const id of Object.keys(prev)) if (!(id in next)) forget(id)
  for (const [id, { data }] of Object.entries(next)) {
    const was = prev[id]?.data
    if (was === data) continue
    if (was?.liveRunnerId && was.liveRunnerId !== data.liveRunnerId) {
      releaseRunner(was.liveRunnerId, id)
    }
    if (live && data.working && !was?.working) adoptIfViewless(data)
  }
}

function followWorkspaces(next: Workspaces, prev: Workspaces) {
  for (const id of Object.keys(prev)) if (!(id in next)) forgetWorkspaceScope(id)
  for (const [id, { data: ws }] of Object.entries(next)) {
    if (prev[id]?.data === ws || !ws.projectId) continue
    recordWorkspaceScope({
      projectId: ws.projectId,
      repoId: ws.repoId,
      wsId: ws.id,
      owningChatId: ws.owningChatId || undefined,
    })
  }
}

/** What follows a chat or workspace record changing: a deletion forgets its client state,
 *  a runner leaving frees its panes, and a chat starting work after the first snapshot
 *  (a live change, not history) is adopted. Returns the stop. */
export function followChatLifecycle(): () => void {
  return entityStore.subscribe((s, prev) => {
    if (s.byKind.chat !== prev.byKind.chat) {
      followChats(s.byKind.chat, prev.byKind.chat, !!prev.loaded.projects)
    }
    if (s.byKind.workspace !== prev.byKind.workspace) {
      followWorkspaces(s.byKind.workspace, prev.byKind.workspace)
    }
  })
}

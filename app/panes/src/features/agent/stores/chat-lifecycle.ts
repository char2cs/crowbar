import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { splitHostKey } from '@/core/stream/host-scope'
import type { ChatEntity } from '@/core/entities/chat-entity'
import { clearExcalidrawEditRequest } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { forgetWorkspaceScope, recordWorkspaceScope } from '@/lib/workspace-scope'

type Chats = EntityState['byKind']['chat']
type Workspaces = EntityState['byKind']['workspace']

const forgetters = new Set<(chatId: string) => void>()

/** A lazily loaded owner of per-chat state joins the forgetting as it loads: unloaded, it holds none. */
export function forgetWithChat(forget: (chatId: string) => void): void {
  forgetters.add(forget)
}

/** A chat the daemon deleted: everything this client keeps for it goes with it. */
function forget(chatId: string) {
  for (const drop of forgetters) drop(chatId)
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

// Records are keyed by host (`entityKey`); what this client keeps per chat is by raw id.
function followChats(next: Chats, prev: Chats, live: boolean) {
  for (const key of Object.keys(prev)) if (!(key in next)) forget(splitHostKey(key).id)
  for (const [key, { data }] of Object.entries(next)) {
    const was = prev[key]?.data
    if (was === data) continue
    if (was?.liveRunnerId && was.liveRunnerId !== data.liveRunnerId) {
      releaseRunner(was.liveRunnerId, data.id)
    }
    if (live && data.working && !was?.working) adoptIfViewless(data)
  }
}

function followWorkspaces(next: Workspaces, prev: Workspaces) {
  for (const key of Object.keys(prev)) {
    if (!(key in next)) forgetWorkspaceScope(splitHostKey(key).id)
  }
  for (const [key, { data: ws }] of Object.entries(next)) {
    if (prev[key]?.data === ws || !ws.projectId) continue
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

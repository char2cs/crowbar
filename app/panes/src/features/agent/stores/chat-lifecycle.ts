import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { splitHostKey } from '@/core/stream/host-scope'
import type { ChatEntity } from '@/core/entities/chat-entity'
import { clearExcalidrawEditRequest } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

type Chats = EntityState['byKind']['chat']

const forgetters = new Set<(chatId: string) => void>()

export function forgetWithChat(forget: (chatId: string) => void): void {
  forgetters.add(forget)
}

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

function adoptIfViewless(chat: ChatEntity) {
  const { panes, paneActions } = windowPaneStore.getState()
  if (chatPaneIndex(panes).has(chat.id) || !chat.projectId) return
  paneActions.adoptBackgroundChat(chat.id, chat.projectId, chat.workspaceId)
}

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

export function followChatLifecycle(): () => void {
  return entityStore.subscribe((s, prev) => {
    if (s.byKind.chat !== prev.byKind.chat) {
      followChats(s.byKind.chat, prev.byKind.chat, !!prev.loaded.projects)
    }
  })
}

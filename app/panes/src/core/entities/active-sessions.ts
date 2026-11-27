import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { splitHostKey } from '@/core/stream/host-scope'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'

export interface ActiveSession {
  hostId: string
  chatId: string
  workspaceId: string
  title: string
  viewers: number
}

type Chats = EntityState['byKind']['chat']
type Presence = EntityState['byKind']['presence']

let memo: { chats: Chats; presence: Presence; list: ActiveSession[] } | undefined

function selectActiveSessions(s: EntityState): ActiveSession[] {
  const { chat: chats, presence } = s.byKind
  if (memo?.chats === chats && memo.presence === presence) return memo.list
  const list: ActiveSession[] = []
  for (const [key, { data }] of Object.entries(chats)) {
    if (!data.liveRunnerId) continue
    list.push({
      hostId: splitHostKey(key).hostId,
      chatId: data.id,
      workspaceId: data.workspaceId ?? '',
      title: data.title || UNTITLED_CHAT_LABEL,
      viewers: presence[key]?.data.viewers ?? 0,
    })
  }
  list.sort(
    (a, b) =>
      a.title.localeCompare(b.title) ||
      a.hostId.localeCompare(b.hostId) ||
      a.chatId.localeCompare(b.chatId),
  )
  memo = { chats, presence, list }
  return list
}

export function selectActiveSession(s: EntityState, chatId: string): ActiveSession | undefined {
  return selectActiveSessions(s).find((session) => session.chatId === chatId)
}

export function viewersLabel(viewers: number): string {
  if (viewers === 0) return 'No viewers'
  return viewers === 1 ? '1 viewer' : `${viewers} viewers`
}

export function useActiveSessionList(): ActiveSession[] {
  return useStore(entityStore, useShallow(selectActiveSessions))
}

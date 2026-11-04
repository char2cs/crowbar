import { useStore } from 'zustand'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { splitHostKey } from '@/core/stream/host-scope'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'

/** A chat whose agent is running now, on any host, and how many connections view it. */
export interface ActiveSession {
  hostId: string
  chatId: string
  workspaceId: string
  title: string
  /** Connections holding the chat's scope on its host, this client's own among them;
   *  0 is an orphan, running with nobody watching. */
  viewers: number
}

type Chats = EntityState['byKind']['chat']
type Presence = EntityState['byKind']['presence']

let memo: { chats: Chats; presence: Presence; list: ActiveSession[] } | undefined

/** Every running chat of every host, by title, each with its viewers: the same array until
 *  a chat or a presence row changes. */
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

/** The running session of `chatId`, if its agent is running. */
export function selectActiveSession(s: EntityState, chatId: string): ActiveSession | undefined {
  return selectActiveSessions(s).find((session) => session.chatId === chatId)
}

/** "2 viewers", or "No viewers" for a running agent nobody is viewing. */
export function viewersLabel(viewers: number): string {
  if (viewers === 0) return 'No viewers'
  return viewers === 1 ? '1 viewer' : `${viewers} viewers`
}

export function useActiveSessionList(): ActiveSession[] {
  return useStore(entityStore, selectActiveSessions)
}

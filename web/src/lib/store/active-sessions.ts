import { useMemo } from 'react'
import { create } from 'zustand'

/**
 * The agents running right now, and how many clients are attached to each.
 *
 * The sidebar tree deliberately carries nothing about processes (see
 * `RepoChatWireDTO`); this is the live half it leaves out. Both facts are the
 * daemon's — a snapshot frame's `liveRunnerId`, and the `attached` frame — and
 * only the chat's identity is kept here: placement and parentage stay where the
 * tree has them, and rows are looked up by id.
 *
 * `scope` is the feed a fact arrived on (a repo or a project home), so a reseed
 * or a closed subscription replaces exactly its own.
 */
export interface LiveSession {
  chatId: string
  workspaceId: string
  title: string
  scope: string
}

export interface ActiveSession extends LiveSession {
  /** Clients attached right now; 0 is an orphan — running with nobody watching. */
  attached: number
}

/** The slice of a chat-feed frame this store reads. */
export interface ActiveSessionFrame {
  chatId?: string
  kind?: string
  attached?: number
  chat?: { id: string; workspaceId: string; title: string; liveRunnerId?: string }
}

/** The slice of a chat-list row this store reads. */
export interface LiveRow {
  id: string
  workspaceId: string
  title: string
  liveRunnerId?: string
}

interface ActiveSessionsState {
  sessions: Record<string, LiveSession>
  attached: Record<string, { scope: string; count: number }>
  applyFrame: (scope: string, frame: ActiveSessionFrame) => void
  /** Replace everything `scope` knew about running chats with a fresh list. */
  seedScope: (scope: string, sessions: readonly LiveSession[]) => void
  /** Forget `scope`'s counts: a reconnected socket restates the live ones. */
  resetAttached: (scope: string) => void
  clearScope: (scope: string) => void
}

export function liveSessionsFromRows(scope: string, rows: readonly LiveRow[]): LiveSession[] {
  const live: LiveSession[] = []
  for (const row of rows) {
    if (row.liveRunnerId) {
      live.push({ chatId: row.id, workspaceId: row.workspaceId, title: row.title, scope })
    }
  }
  return live
}

function without<T>(record: Record<string, T>, keep: (value: T) => boolean): Record<string, T> {
  return Object.fromEntries(Object.entries(record).filter(([, value]) => keep(value)))
}

export const useActiveSessionsStore = create<ActiveSessionsState>()((set) => ({
  sessions: {},
  attached: {},
  applyFrame: (scope, frame) => {
    const { chatId } = frame
    if (!chatId) return
    if (frame.kind === 'attached') {
      set((s) => {
        const attached = { ...s.attached }
        if (frame.attached) attached[chatId] = { scope, count: frame.attached }
        else delete attached[chatId]
        return { attached }
      })
      return
    }
    if (frame.kind === 'deleted') {
      set((s) => ({
        sessions: without(s.sessions, (v) => v.chatId !== chatId),
        attached: Object.fromEntries(Object.entries(s.attached).filter(([id]) => id !== chatId)),
      }))
      return
    }
    const { chat } = frame
    if (!chat) return
    set((s) => {
      const sessions = { ...s.sessions }
      if (chat.liveRunnerId) {
        sessions[chatId] = { chatId, workspaceId: chat.workspaceId, title: chat.title, scope }
      } else delete sessions[chatId]
      return { sessions }
    })
  },
  seedScope: (scope, list) =>
    set((s) => ({
      sessions: {
        ...without(s.sessions, (v) => v.scope !== scope),
        ...Object.fromEntries(list.map((session) => [session.chatId, session])),
      },
    })),
  resetAttached: (scope) =>
    set((s) => ({ attached: without(s.attached, (v) => v.scope !== scope) })),
  clearScope: (scope) =>
    set((s) => ({
      sessions: without(s.sessions, (v) => v.scope !== scope),
      attached: without(s.attached, (v) => v.scope !== scope),
    })),
}))

/** "2 viewers", or "No viewers" for a running agent nobody is attached to. */
export function viewersLabel(attached: number): string {
  if (attached === 0) return 'No viewers'
  return attached === 1 ? '1 viewer' : `${attached} viewers`
}

/** Running chats, by title, each with its viewer count. */
function selectActiveSessions(
  state: Pick<ActiveSessionsState, 'sessions' | 'attached'>,
): ActiveSession[] {
  return Object.values(state.sessions)
    .map((session) => ({ ...session, attached: state.attached[session.chatId]?.count ?? 0 }))
    .sort((a, b) => a.title.localeCompare(b.title) || a.chatId.localeCompare(b.chatId))
}

export function useActiveSessionList(): ActiveSession[] {
  const sessions = useActiveSessionsStore((s) => s.sessions)
  const attached = useActiveSessionsStore((s) => s.attached)
  return useMemo(() => selectActiveSessions({ sessions, attached }), [sessions, attached])
}

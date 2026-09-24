import { useEffect, useSyncExternalStore } from 'react'
import { getChat } from '@/features/agent/api/agent-api'
import {
  getWorkspaceStore,
  resolveWorkspaceIdForChat,
  subscribeWorkspaceStores,
} from '@/features/workspace/stores/workspace-store-registry'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isNotFoundError } from '@/lib/api'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useConnectionStore } from '@/lib/ws/connection-store'
import type { RecentsChat } from './use-recents-chat'

type Fetched =
  | { status: 'pending' }
  | { status: 'retry'; projectId: string; hint: string }
  | { status: 'gone' }
  | { status: 'done'; chat: RecentsChat }

const fetched = new Map<string, Fetched>()
const listeners = new Set<() => void>()
let unwatchRetrySignals: (() => void) | null = null

function publish(chatId: string, entry: Fetched): void {
  fetched.set(chatId, entry)
  if (entry.status === 'retry') watchRetrySignals()
  for (const listener of listeners) listener()
}

function subscribeFetched(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Re-ask every transiently failed read. */
function retryFailed(): void {
  const due = [...fetched].filter(
    (e): e is [string, Extract<Fetched, { status: 'retry' }>] => e[1].status === 'retry',
  )
  unwatchRetrySignals?.()
  unwatchRetrySignals = null
  for (const [chatId, entry] of due) {
    fetched.delete(chatId)
    requestChat(chatId, entry.projectId, entry.hint)
  }
}

// The streams' own recovery signals: a reconnect bumps the repo tree signal,
// and a socket reopening flips the connection back to 'connected'.
function watchRetrySignals(): void {
  if (unwatchRetrySignals) return
  const unFolders = useFolderSignalStore.subscribe((s) => s.generations, retryFailed)
  const unConnection = useConnectionStore.subscribe((s, prev) => {
    if (s.status === 'connected' && prev.status !== 'connected') retryFailed()
  })
  unwatchRetrySignals = () => {
    unFolders()
    unConnection()
  }
}

/** The mounts that can answer for a chat in `projectId`. */
function candidateWorkspaces(projectId: string): string[] {
  const out = [getHomeWorkspaceId(projectId) ?? '']
  for (const repo of useSidebarStore.getState().repos) {
    if (repo.projectId !== projectId) continue
    out.push(repo.defaultWorkspaceId ?? '', ...repo.workspaces.map((w) => w.id))
  }
  return [...new Set(out.filter(Boolean))]
}

/**
 * One read per chat, through its owner's mount when known (a 404 there is a
 * deletion), else through every project mount (only all of them 404ing is).
 * A deleted chat is forgotten; any other failure waits for a recovery signal.
 */
function requestChat(chatId: string, projectId: string, hint: string): void {
  if (fetched.has(chatId)) return
  fetched.set(chatId, { status: 'pending' })
  void (async () => {
    const mounts = hint ? [hint] : candidateWorkspaces(projectId)
    let notFound = 0
    for (const wsId of mounts) {
      try {
        const chat = await getChat(wsId, chatId)
        publish(chatId, {
          status: 'done',
          chat: { id: chat.id, title: chat.title, workspaceId: chat.workspaceId, working: false },
        })
        return
      } catch (err) {
        if (isNotFoundError(err)) notFound++
      }
    }
    if (mounts.length > 0 && notFound === mounts.length) {
      publish(chatId, { status: 'gone' })
      windowPaneStore.getState().paneActions.forgetChat(chatId)
      return
    }
    publish(chatId, { status: 'retry', projectId, hint })
  })()
}

export function _resetRecentsChatFallbackForTests(): void {
  fetched.clear()
  unwatchRetrySignals?.()
  unwatchRetrySignals = null
}

const noSubscribe = () => () => {}

/**
 * What a Recents row draws while `useRecentsChat` has no record for its chat:
 * any registered store's copy, else one fetched through the owner; `pending`
 * until one arrives. Inert while `enabled` is false.
 */
export function useRecentsChatFallback(
  chatId: string,
  projectId: string,
  workspaceHint: string,
  enabled: boolean,
): { chat: RecentsChat; pending: boolean } {
  const stored = useSyncExternalStore(enabled ? subscribeWorkspaceStores : noSubscribe, () => {
    if (!enabled) return undefined
    const wsId = resolveWorkspaceIdForChat(chatId)
    return wsId
      ? getWorkspaceStore(wsId)
          ?.getState()
          .agentChats.chats.find((c) => c.id === chatId)
      : undefined
  })
  const entry = useSyncExternalStore(enabled ? subscribeFetched : noSubscribe, () =>
    enabled ? fetched.get(chatId) : undefined,
  )

  useEffect(() => {
    if (enabled && !stored) requestChat(chatId, projectId, workspaceHint)
  }, [enabled, stored, chatId, projectId, workspaceHint])

  if (stored) {
    return {
      chat: { id: chatId, title: stored.title, workspaceId: stored.workspaceId, working: false },
      pending: false,
    }
  }
  if (entry?.status === 'done') return { chat: entry.chat, pending: false }
  return {
    chat: { id: chatId, title: '', workspaceId: workspaceHint, working: false },
    pending: true,
  }
}

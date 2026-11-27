import { useSyncExternalStore } from 'react'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

export interface WorkspaceScope {
  projectId: string
  repoId: string
  wsId: string
  owningChatId?: string
}

function notifyScopeListeners(wsId: string): void {
  const listeners = useWorkspaceScopeStore.getState().listeners[wsId]
  if (!listeners) return
  let failure: { error: unknown } | null = null
  for (const listener of listeners) {
    try {
      listener()
    } catch (error) {
      failure ??= { error }
    }
  }
  if (failure) throw failure.error
}

export function subscribeToWorkspaceScope(wsId: string, callback: () => void): () => void {
  const { listeners } = useWorkspaceScopeStore.getState()
  useWorkspaceScopeStore.setState({
    listeners: { ...listeners, [wsId]: new Set(listeners[wsId]).add(callback) },
  })
  return () => {
    const current = useWorkspaceScopeStore.getState().listeners
    const remaining = new Set(current[wsId])
    remaining.delete(callback)
    const next = { ...current, [wsId]: remaining }
    if (remaining.size === 0) delete next[wsId]
    useWorkspaceScopeStore.setState({ listeners: next })
  }
}

function mergeScope(scope: WorkspaceScope): WorkspaceScope {
  const prev = useWorkspaceScopeStore.getState().scopes[scope.wsId]
  const owningChatId = scope.owningChatId || prev?.owningChatId
  return owningChatId ? { ...scope, owningChatId } : { ...scope }
}

const sameScope = (a: WorkspaceScope | undefined, b: WorkspaceScope): boolean =>
  a !== undefined &&
  a.projectId === b.projectId &&
  a.repoId === b.repoId &&
  a.owningChatId === b.owningChatId

function writeScope(scope: WorkspaceScope): void {
  const next = mergeScope(scope)
  const { scopes } = useWorkspaceScopeStore.getState()
  if (sameScope(scopes[scope.wsId], next)) return
  useWorkspaceScopeStore.setState({ scopes: { ...scopes, [scope.wsId]: next } })
  notifyScopeListeners(scope.wsId)
}

export function setWorkspaceScope(scope: WorkspaceScope): void {
  writeScope(scope)
}

export function recordWorkspaceScope(scope: WorkspaceScope): void {
  writeScope(scope)
}

export function forgetWorkspaceScope(wsId: string): void {
  const { scopes } = useWorkspaceScopeStore.getState()
  if (!(wsId in scopes)) return
  const remaining = { ...scopes }
  delete remaining[wsId]
  useWorkspaceScopeStore.setState({ scopes: remaining })
  notifyScopeListeners(wsId)
}

const IDE_ROUTE = /\/ide\/([^/]+)\/([^/]+)\/([^/]+)/

export function recordWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const scope = parseWorkspaceScopeFromPath(pathname)
  if (scope) setWorkspaceScope(scope)
  return scope
}

function parseWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const match = pathname.match(IDE_ROUTE)
  if (!match) return null
  return { projectId: match[1], repoId: match[2], wsId: match[3] }
}

export function getWorkspaceScope(wsId: string): WorkspaceScope | null {
  return useWorkspaceScopeStore.getState().scopes[wsId] ?? null
}

export function getOwningChatId(wsId: string): string | null {
  return getWorkspaceScope(wsId)?.owningChatId || null
}

export function getWorkspaceIdOwnedByChat(chatId: string): string | null {
  if (!chatId) return null
  for (const scope of Object.values(useWorkspaceScopeStore.getState().scopes))
    if (scope.owningChatId === chatId) return scope.wsId
  return null
}

export function useOwningChatId(wsId: string): string | null {
  return useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )
}

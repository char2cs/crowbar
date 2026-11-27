import { useSyncExternalStore } from 'react'
import { entityStore, type EntityState } from '@/core/entities/entity-store'

export interface HomeWorkspaceState {
  wsId: string | null
  owningChatId: string | null
  localPath: string | null
  error: boolean
}

const UNRESOLVED: HomeWorkspaceState = {
  wsId: null,
  owningChatId: null,
  localPath: null,
  error: false,
}
const MISSING: HomeWorkspaceState = { ...UNRESOLVED, error: true }

let memo:
  { workspaces: unknown; synced: boolean; homes: Map<string, HomeWorkspaceState> } | undefined

function homesOf(s: EntityState): ReadonlyMap<string, HomeWorkspaceState> {
  const workspaces = s.byKind.workspace
  const synced = !!s.loaded.projects
  if (memo?.workspaces === workspaces && memo.synced === synced) return memo.homes
  const homes = new Map<string, HomeWorkspaceState>()
  for (const { data: ws } of Object.values(workspaces)) {
    if (ws.repoId || !ws.projectId) continue
    homes.set(ws.projectId, {
      wsId: ws.id,
      owningChatId: ws.owningChatId || null,
      localPath: ws.localPath || null,
      error: false,
    })
  }
  memo = { workspaces, synced, homes }
  return homes
}

function stateOf(s: EntityState, projectId: string | null): HomeWorkspaceState {
  if (!projectId) return UNRESOLVED
  const home = homesOf(s).get(projectId)
  if (home) return home
  return s.loaded.projects && projectId in s.byKind.project ? MISSING : UNRESOLVED
}

const read = (projectId: string) => stateOf(entityStore.getState(), projectId)

export function getKnownHomeWorkspaceIds(): string[] {
  return [...homesOf(entityStore.getState()).values()].flatMap((h) => (h.wsId ? [h.wsId] : []))
}

export function getHomeWorkspaceId(projectId: string): string | null {
  return read(projectId).wsId
}

export function getHomeOwningChatId(projectId: string): string | null {
  return read(projectId).owningChatId
}

export function useHomeWorkspaceStates(): ReadonlyMap<string, HomeWorkspaceState> {
  return useSyncExternalStore(entityStore.subscribe, () => homesOf(entityStore.getState()))
}

export function useHomeWorkspaceState(projectId: string | null): HomeWorkspaceState {
  return useSyncExternalStore(entityStore.subscribe, () =>
    stateOf(entityStore.getState(), projectId),
  )
}

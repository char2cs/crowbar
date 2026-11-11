import { useSyncExternalStore } from 'react'
import { entityStore, type EntityState } from '@/core/entities/entity-store'

/** A project's home workspace as the projects scope carries it. */
export interface HomeWorkspaceState {
  wsId: string | null
  owningChatId: string | null
  localPath: string | null
  /** The projects scope has synced and names no home for the project. */
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

/** Every project's home, the workspace its repo-less row is (same map until one changes). */
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

/** Every home-workspace id known (any project), so retention never prunes one for being
 *  absent from the repos' workspaces. */
export function getKnownHomeWorkspaceIds(): string[] {
  return [...homesOf(entityStore.getState()).values()].flatMap((h) => (h.wsId ? [h.wsId] : []))
}

/** `projectId`'s home-workspace id, or `null` before the projects scope carried it. */
export function getHomeWorkspaceId(projectId: string): string | null {
  return read(projectId).wsId
}

/** The chat that owns `projectId`'s home workspace, or `null` before it is known. */
export function getHomeOwningChatId(projectId: string): string | null {
  return read(projectId).owningChatId
}

/** `projectId`'s home workspace's own on-disk directory, or `null` before it is known. */
export function getHomeWorkspacePath(projectId: string): string | null {
  return read(projectId).localPath
}

export function useHomeWorkspaceStates(): ReadonlyMap<string, HomeWorkspaceState> {
  return useSyncExternalStore(entityStore.subscribe, () => homesOf(entityStore.getState()))
}

/** `projectId`'s home workspace (the stable "unresolved" sentinel before it is known). */
export function useHomeWorkspaceState(projectId: string | null): HomeWorkspaceState {
  return useSyncExternalStore(entityStore.subscribe, () =>
    stateOf(entityStore.getState(), projectId),
  )
}

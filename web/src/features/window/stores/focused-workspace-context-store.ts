import { create } from 'zustand'

/**
 * The workspace the focused pane is scoped to — the single authority for the
 * sidebar panel (its tab strip, each tab's availability, each tab body) and
 * any future panel extension. Never read the route to scope panel content.
 */
export interface FocusedWorkspaceContext {
  projectId: string | null
  workspaceId: string | null
  isProjectHome: boolean
  /** Null for project home, or while the workspace's repo is unknown. */
  repoId: string | null
  /** The workspace's git working tree; null whenever `repoId` is. */
  repoPath: string | null
  /** Directory the Files tab roots at ('' while unresolved). */
  rootPath: string
}

export const EMPTY_FOCUSED_WORKSPACE_CONTEXT: FocusedWorkspaceContext = {
  projectId: null,
  workspaceId: null,
  isProjectHome: false,
  repoId: null,
  repoPath: null,
  rootPath: '',
}

const KEYS = Object.keys(EMPTY_FOCUSED_WORKSPACE_CONTEXT) as (keyof FocusedWorkspaceContext)[]

export const useFocusedWorkspaceContextStore = create<FocusedWorkspaceContext>(() => ({
  ...EMPTY_FOCUSED_WORKSPACE_CONTEXT,
}))

/** Writes only when a field actually changed, so narrow selectors stay quiet. */
export function publishFocusedWorkspaceContext(next: FocusedWorkspaceContext): void {
  const prev = useFocusedWorkspaceContextStore.getState()
  if (KEYS.every((k) => prev[k] === next[k])) return
  useFocusedWorkspaceContextStore.setState({ ...next })
}

export function getFocusedWorkspaceContext(): FocusedWorkspaceContext {
  const s = useFocusedWorkspaceContextStore.getState()
  return {
    projectId: s.projectId,
    workspaceId: s.workspaceId,
    isProjectHome: s.isProjectHome,
    repoId: s.repoId,
    repoPath: s.repoPath,
    rootPath: s.rootPath,
  }
}

/** Git (and any repo-scoped tab) needs a real repo workspace. */
export function hasRepoWorkspace(ctx: Pick<FocusedWorkspaceContext, 'repoId' | 'workspaceId'>) {
  return ctx.workspaceId != null && ctx.repoId != null
}

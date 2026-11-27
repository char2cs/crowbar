import { create } from 'zustand'

export interface FocusedWorkspaceContext {
  projectId: string | null
  workspaceId: string | null
  isProjectHome: boolean
  repoId: string | null
  repoPath: string | null
  rootPath: string
}

const EMPTY_FOCUSED_WORKSPACE_CONTEXT: FocusedWorkspaceContext = {
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

export function hasRepoWorkspace(ctx: Pick<FocusedWorkspaceContext, 'repoId' | 'workspaceId'>) {
  return ctx.workspaceId != null && ctx.repoId != null
}

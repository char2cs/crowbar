import { create } from 'zustand'
import type { WorkspaceScope } from '@/lib/workspace-scope'

interface WorkspaceScopeState {
  /** Each workspace's owning project+repo (and chat), keyed by wsId. */
  scopes: Record<string, WorkspaceScope>
  /** Who to tell when a workspace's scope is written, keyed by wsId. */
  listeners: Record<string, ReadonlySet<() => void>>
}

/** The workspace scopes and their listeners; dependency-free so the URL builders resolve scopes without pulling in the editor/Monaco workspace-store graph. */
export const useWorkspaceScopeStore = create<WorkspaceScopeState>(() => ({
  scopes: {},
  listeners: {},
}))

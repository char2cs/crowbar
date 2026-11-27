import { create } from 'zustand'
import type { WorkspaceScope } from '@/lib/workspace-scope'

interface WorkspaceScopeState {
  scopes: Record<string, WorkspaceScope>
  listeners: Record<string, ReadonlySet<() => void>>
}

export const useWorkspaceScopeStore = create<WorkspaceScopeState>(() => ({
  scopes: {},
  listeners: {},
}))

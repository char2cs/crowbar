import { useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import {
  getAllActiveWorkspaceIds,
  subscribeWorkspaceRegistry,
} from '@/features/workspace/stores/workspace-store-registry'

const countOpen = () => getAllActiveWorkspaceIds().length

function countWorking(s: EntityState): number {
  let n = 0
  for (const entry of Object.values(s.byKind.chat)) if (entry.data.working) n++
  return n
}

export function useOpenWorkspaceCount(): number {
  return useSyncExternalStore(subscribeWorkspaceRegistry, countOpen)
}

export function useWorkingChatCount(): number {
  return useStore(entityStore, countWorking)
}

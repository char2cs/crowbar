import { useSyncExternalStore } from 'react'
import {
  getAllActiveWorkspaceIds,
  getWorkspaceStore,
  subscribeWorkspaceRegistry,
  subscribeWorkspaceStores,
} from '@/features/workspace/stores/workspace-store-registry'

const countOpen = () => getAllActiveWorkspaceIds().length

function countWorking(): number {
  let n = 0
  for (const wsId of getAllActiveWorkspaceIds()) {
    const working = getWorkspaceStore(wsId)?.getState().agentChats.working ?? {}
    for (const flag of Object.values(working)) if (flag) n++
  }
  return n
}

/** Workspaces with a live store — the host keeps one iff it is in a view. */
export function useOpenWorkspaceCount(): number {
  return useSyncExternalStore(subscribeWorkspaceRegistry, countOpen)
}

/** Chats mid-turn in the open workspaces; a closed one streams no `working`. */
export function useWorkingChatCount(): number {
  return useSyncExternalStore(subscribeWorkspaceStores, countWorking)
}

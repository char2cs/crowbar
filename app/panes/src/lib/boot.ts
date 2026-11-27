import {
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredPanesOnceSynced,
} from '@/lib/persistence/hydrate'
import { preloadAgentChatPane } from '@/features/panes/lib/chat-surface-loader'
import { holdShownChatFeeds } from '@/features/panes/lib/shown-chat-feeds'
import { showingLayout } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { markStart } from '@/lib/perf/instrumentation'
import {
  hydrateHostPlacements,
  startHostPlacementCache,
} from '@/features/hosts/lib/host-placements'
import { retireOrphanedStorage } from '@/lib/persistence/retired-storage'
import { hostsRegistered } from '@/features/hosts/lib/host-registration'
import { hasPairedRemote, useHostConfigStore } from '@/features/hosts/stores/host-config-store'

export async function hydrateCriticalStores(): Promise<void> {
  void retireOrphanedStorage()
  const remoteHosts = prepareRemoteHosts()
  try {
    await Promise.all([hydrateWindowPaneLayout(), hydrateHostPlacements()])
    startHostPlacementCache()
  } catch (err) {
    if (remoteHosts) await remoteHosts
    throw err
  }
  const chatSurface = showsChat() ? preloadAgentChatPane() : undefined
  void holdShownChatFeeds()
  if (chatSurface) markStart('chat:first-row')
  if (remoteHosts) await remoteHosts
  placeRestoredPanesOnceSynced()
  await hydrateSidebar()
  try {
    await chatSurface
  } catch {
    return
  }
}

function prepareRemoteHosts(): Promise<void> | undefined {
  if (!hasPairedRemote(useHostConfigStore.getState())) return undefined
  return hostsRegistered()
}

function showsChat(): boolean {
  const state = windowPaneStore.getState()
  return getAllLeafIds(showingLayout(state)).some((id) => state.panes[id]?.chatId)
}

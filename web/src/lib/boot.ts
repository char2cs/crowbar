import {
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredPanesOnceSynced,
} from '@/lib/persistence/hydrate'
import { preloadAgentChatPane } from '@/features/panes/lib/chat-surface-loader'
import { showingLayout } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { markStart } from '@/lib/perf/instrumentation'
import { retireOrphanedStorage } from '@/lib/persistence/retired-storage'
import { hostsRegistered } from '@/features/hosts/lib/host-registration'
import { hasPairedRemote, useHostConfigStore } from '@/features/hosts/stores/host-config-store'

// Hydrates this window's pane/buffer layout and sidebar UI from IndexedDB before first render: a
// surface swapped from empty defaults a frame later trips on a workspace store not yet armed for
// its pane. The tree itself is the projects scope's, drawn as a skeleton until its snapshot.
export async function hydrateCriticalStores(): Promise<void> {
  void retireOrphanedStorage()
  const remoteHosts = prepareRemoteHosts()
  try {
    await hydrateWindowPaneLayout()
  } catch (err) {
    // main.tsx renders even after a failed boot: render must not overtake the registration.
    if (remoteHosts) await remoteHosts
    throw err
  }
  // The chat surface chunk loads beside the IndexedDB reads below, and render
  // waits for it, so the restored chat mounts in the first commit.
  const chatSurface = showsChat() ? preloadAgentChatPane() : undefined
  // Layout hydration to the first transcript row shown; AgentTranscript closes it.
  if (chatSurface) markStart('chat:first-row')
  if (remoteHosts) await remoteHosts
  // Restored members stay unplaced until the projects scope answers for them.
  placeRestoredPanesOnceSynced()
  await hydrateSidebar()
  try {
    await chatSurface
  } catch {
    return // the pane's own lazy load retries the chunk and shows its failure
  }
}

/** A paired install's hosts registered with the desktop bridge before a restored pane
 *  dials one. Undefined where no remote was ever paired, so that boot does exactly what it did. */
function prepareRemoteHosts(): Promise<void> | undefined {
  if (!hasPairedRemote(useHostConfigStore.getState())) return undefined
  return hostsRegistered()
}

function showsChat(): boolean {
  const state = windowPaneStore.getState()
  return getAllLeafIds(showingLayout(state)).some((id) => state.panes[id]?.chatId)
}

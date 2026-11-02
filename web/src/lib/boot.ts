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

/**
 * Hydrates this window's pane/buffer layout and sidebar UI from IndexedDB, not the
 * network, before the first render: a surface that mounts against empty defaults and
 * is swapped a frame later trips on a workspace store not yet armed for its pane.
 * The tree itself is the projects scope's, drawn as a skeleton until its snapshot.
 */
export async function hydrateCriticalStores(): Promise<void> {
  void retireOrphanedStorage()
  await hydrateWindowPaneLayout()
  // The chat surface chunk loads beside the IndexedDB reads below, and render
  // waits for it, so the restored chat mounts in the first commit.
  const chatSurface = showsChat() ? preloadAgentChatPane() : undefined
  // Layout hydration to the first transcript row shown; AgentTranscript closes it.
  if (chatSurface) markStart('chat:first-row')
  // Restored members stay unplaced until the projects scope answers for them.
  placeRestoredPanesOnceSynced()
  await hydrateSidebar()
  try {
    await chatSurface
  } catch {
    return // the pane's own lazy load retries the chunk and shows its failure
  }
}

function showsChat(): boolean {
  const state = windowPaneStore.getState()
  return getAllLeafIds(showingLayout(state)).some((id) => state.panes[id]?.chatId)
}

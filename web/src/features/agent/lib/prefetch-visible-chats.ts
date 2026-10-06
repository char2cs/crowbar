import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { showingLayout } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getWorkspaceScope } from '@/lib/workspace-scope'

/**
 * Starts the first-page fetch for every chat on screen, so it overlaps the
 * chat surface's chunk load instead of waiting for the view to mount. Called
 * once the restored layout is in the store, before React renders.
 *
 * The messages URL needs the workspace's project/repo scope, which the sidebar
 * tree records when it is seeded from its cache. A chat whose scope is not
 * known is left to its own view; never blocks render.
 */
export async function prefetchVisibleChats(): Promise<void> {
  const state = windowPaneStore.getState()
  const loads: Promise<void>[] = []
  for (const paneId of getAllLeafIds(showingLayout(state))) {
    const { chatId, workspaceId } = state.panes[paneId] ?? {}
    if (!chatId || !workspaceId) continue
    if (!getWorkspaceScope(workspaceId)) continue
    loads.push(load(workspaceId, chatId))
  }
  await Promise.all(loads)
}

async function load(wsId: string, chatId: string): Promise<void> {
  const { loadInitial, evict } = chatLedgerStore.getState()
  await loadInitial(wsId, chatId)
  // A failed prefetch must not show its error before the view retries.
  if (chatLedgerStore.getState().ledgers[chatId]?.status === 'error') evict(chatId)
}

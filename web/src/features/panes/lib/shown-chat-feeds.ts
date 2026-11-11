import type { attachFeed } from '@/features/agent/stores/chat-feed'
import { showingLayout } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { chatHostOf, subscribeHostResolution } from '@/lib/host-resolve'

const held = new Map<string, () => void>()
let started: Promise<void> | null = null

/** Holds the feed of every chat the window shows, on its host once that resolves. */
function holdShown(attach: typeof attachFeed) {
  const state = windowPaneStore.getState()
  const shown = new Map<string, [hostId: string, chatId: string]>()
  for (const paneId of getAllLeafIds(showingLayout(state))) {
    const chatId = state.panes[paneId]?.chatId
    const hostId = chatId ? chatHostOf(chatId) : null
    if (chatId && hostId) shown.set(`${hostId}\n${chatId}`, [hostId, chatId])
  }
  for (const [key, release] of held) {
    if (shown.has(key)) continue
    release()
    held.delete(key)
  }
  for (const [key, [hostId, chatId]] of shown) {
    if (!held.has(key)) held.set(key, attach(hostId, chatId))
  }
}

/** A shown chat subscribes as soon as the layout and its host are known, not once its
 *  view's chunk has loaded and mounted: its rows replay while that chunk (and, beside it, this
 *  feed code, kept out of the boot bundle) evaluates. */
export function holdShownChatFeeds(): Promise<void> {
  started ??= import('@/features/agent/stores/chat-feed').then(({ attachFeed }) => {
    const hold = () => holdShown(attachFeed)
    hold()
    windowPaneStore.subscribe(hold)
    subscribeHostResolution(undefined, hold)
  })
  return started
}

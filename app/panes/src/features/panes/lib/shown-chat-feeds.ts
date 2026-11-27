import type { attachFeed } from '@/features/agent/stores/chat-feed'
import { showingLayout } from '@/features/panes/lib/view-state'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { subscribeHostResolution } from '@/lib/host-resolve'
import { feedHostOf } from '@/features/hosts/lib/host-resolve-cached'

const held = new Map<string, () => void>()
let started: Promise<void> | null = null

function holdShown(attach: typeof attachFeed) {
  const state = windowPaneStore.getState()
  const shown = new Map<string, [hostId: string, chatId: string]>()
  for (const paneId of getAllLeafIds(showingLayout(state))) {
    const chatId = state.panes[paneId]?.chatId
    const hostId = chatId ? feedHostOf(chatId) : null
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

export function holdShownChatFeeds(): Promise<void> {
  started ??= import('@/features/agent/stores/chat-feed').then(({ attachFeed }) => {
    const hold = () => holdShown(attachFeed)
    hold()
    windowPaneStore.subscribe(hold)
    subscribeHostResolution(undefined, hold)
  })
  return started
}

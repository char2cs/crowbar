import { useEffect } from 'react'
import { useStore } from 'zustand'
import { selectAgentChat, useSelect } from '@/core/entities/select'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

const PRODUCT = 'Crowbar'

/** Names the focused chat in the window title, so tabs, the dock and window switchers tell
 *  windows apart. */
export function useDocumentTitle(): void {
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? null)
  const title = useSelect((s) => (chatId ? selectAgentChat(s, chatId)?.title : undefined))
  useEffect(() => {
    document.title = title ? `${title} — ${PRODUCT}` : PRODUCT
  }, [title])
}

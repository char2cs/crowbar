import type { RefObject } from 'react'
import { usePaneViewPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { PaneGroup } from '@/features/panes/types/pane'

/**
 * How a pane arranges its two views (spec §7.2) — the chat view and the editor
 * view (everything `editorTabIds` holds): side by side, stacked, or collapsed
 * to tabs. Geometry comes from `usePaneViewPresentation` (measured on
 * `viewsContainerRef`); the rest is derived from the pane record. Both views
 * stay mounted in every state; this only says which one is hidden.
 */
export function usePanePresentation(
  pane: Pick<PaneGroup, 'chatId' | 'editorOpen' | 'editorTabIds' | 'chatSelected'>,
  sidebarPosition: 'left' | 'right',
  viewsContainerRef: RefObject<HTMLDivElement | null>,
) {
  const presentation = usePaneViewPresentation(pane.editorOpen, viewsContainerRef)
  const hasChat = Boolean(pane.chatId)
  // A chat with no editor tabs has no IDE sector: chat only, whatever the
  // split toggle or the room says.
  const chatFillsPane = hasChat && pane.editorTabIds.length === 0
  // Collapsed with real tabs: the chat is "just another tab". Read as
  // `!== false` — a layout saved before the field existed shows the chat.
  const chatSelectedInTabsMode = pane.chatSelected !== false
  const showChatTab = hasChat && !chatFillsPane && presentation === 'tabs'
  // Both boxes on screen at once: each keeps its own header.
  const chatVisibleAlongsideEditor = hasChat && !chatFillsPane && presentation !== 'tabs'
  // One header spans the pane only when a single surface fills it.
  const showTopLevelHeader = !hasChat || chatFillsPane || presentation === 'tabs'
  const editorViewHidden =
    hasChat &&
    (chatFillsPane || (presentation === 'tabs' && (!showChatTab || chatSelectedInTabsMode)))
  // The one state that hides the chat: collapsed, with a real tab selected.
  const chatViewHidden = showChatTab && !chatSelectedInTabsMode
  const isStacked = hasChat && presentation === 'stacked' && !chatFillsPane
  // The chat sits beside the sidebar; stacked always keeps it on top.
  const chatIsFirst = presentation === 'stacked' || sidebarPosition !== 'right'
  // The editor view's edge that faces the chat (only meaningful side by side
  // or stacked).
  const editorFacingChatEdge: 'top' | 'left' | 'right' =
    presentation === 'stacked' ? 'top' : chatIsFirst ? 'left' : 'right'
  return {
    presentation,
    chatFillsPane,
    showChatTab,
    chatVisibleAlongsideEditor,
    showTopLevelHeader,
    editorViewHidden,
    chatViewHidden,
    isStacked,
    chatIsFirst,
    editorFacingChatEdge,
  }
}

import type { RefObject } from 'react'
import { usePaneViewPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { PaneGroup } from '@/features/panes/types/pane'

export function usePanePresentation(
  pane: Pick<PaneGroup, 'chatId' | 'editorOpen' | 'editorTabIds' | 'chatSelected'>,
  sidebarPosition: 'left' | 'right',
  viewsContainerRef: RefObject<HTMLDivElement | null>,
) {
  const presentation = usePaneViewPresentation(pane.editorOpen, viewsContainerRef)
  const hasChat = Boolean(pane.chatId)
  const chatFillsPane = hasChat && pane.editorTabIds.length === 0 && !pane.editorOpen
  const chatSelectedInTabsMode = pane.chatSelected !== false
  const showChatTab = hasChat && !chatFillsPane && presentation === 'tabs'
  const chatVisibleAlongsideEditor = hasChat && !chatFillsPane && presentation !== 'tabs'
  const showTopLevelHeader = !hasChat || chatFillsPane || presentation === 'tabs'
  const editorViewHidden =
    hasChat &&
    (chatFillsPane || (presentation === 'tabs' && (!showChatTab || chatSelectedInTabsMode)))
  const chatViewHidden = showChatTab && !chatSelectedInTabsMode
  const isStacked = hasChat && presentation === 'stacked' && !chatFillsPane
  const chatIsFirst = presentation === 'stacked' || sidebarPosition !== 'right'
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

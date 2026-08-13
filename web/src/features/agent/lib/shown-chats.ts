import type { LayoutNode } from '@/features/panes/types/pane'

/**
 * Which chats are ON SCREEN — the only thing that lights a row in the Chats
 * panel.
 *
 * A chat is on screen when it is the ACTIVE TAB OF A PANE THE LAYOUT RENDERS.
 * The panel used to light any chat holding a buffer, so a chat sitting in a
 * background tab stayed lit with nothing on screen to justify it, and the
 * highlight stopped meaning "this is what you are looking at".
 *
 * Two states, never three. A chat parked in a tab you cannot see gets no mark of
 * its own: it is not on screen, and a column of half-signals is harder to read
 * than a column carrying one.
 *
 * ── Why the LAYOUT and not `panes` ──────────────────────────────────────────
 * The store holds panes nothing renders. `bottomLayout` and its BOTTOM_PANE_ID
 * exist in the pane record and no component draws them (asserted in
 * workspace-view.test.tsx). Iterating `Object.values(panes)` would therefore
 * light a row for a chat in a pane the user cannot see — reintroducing the exact
 * defect this replaces, in a way no amount of pane-level care would catch.
 * `rootLayout`'s leaves ARE the set of panes on screen, so they are what this
 * reads.
 */

/** Just enough of a buffer to find the chat behind a tab. */
interface BufferLike {
  id: string
  type: string
  chatId?: string
}

/** Just enough of a pane to answer "what is this pane showing right now". */
interface PaneLike {
  bufferIds: string[]
  activeBufferId: string | null
}

/** The pane ids a layout actually puts on screen. */
function renderedPaneIds(node: LayoutNode, out: Set<string>): Set<string> {
  if (node.type === 'pane') {
    out.add(node.id)
    return out
  }
  renderedPaneIds(node.first, out)
  renderedPaneIds(node.second, out)
  return out
}

export function shownChatIds(
  buffers: readonly BufferLike[],
  panes: Readonly<Record<string, PaneLike>>,
  rootLayout: LayoutNode,
): Set<string> {
  const chatByBuffer = new Map<string, string>()
  for (const b of buffers) {
    if (b.type === 'agentChat' && b.chatId) chatByBuffer.set(b.id, b.chatId)
  }

  const onScreen = renderedPaneIds(rootLayout, new Set())
  const shown = new Set<string>()
  for (const [paneId, pane] of Object.entries(panes)) {
    if (!onScreen.has(paneId)) continue
    const active = pane.activeBufferId
    // `bufferIds.includes` is not redundant with the null check: a close can
    // leave activeBufferId pointing at a tab the pane no longer holds, and a
    // stale pointer is not something on screen.
    if (!active || !pane.bufferIds.includes(active)) continue
    const chatId = chatByBuffer.get(active)
    if (chatId) shown.add(chatId)
  }
  return shown
}

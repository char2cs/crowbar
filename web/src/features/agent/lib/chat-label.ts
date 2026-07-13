/**
 * What a chat is called before anything has named it.
 *
 * A chat is born untitled and stays that way until the agent titles itself (via
 * `crowbar chat rename`), the first prompt derives one, or the user renames it. Every
 * surface that can show a chat — the sidebar row, the drag ghost, the pane tab — needs
 * the same word for that state, and they must not drift apart: the pane tab and the
 * sidebar naming the same chat differently is a bug the user reads as "these are two
 * different chats".
 *
 * This matters more than it used to. A pane's tab now FOLLOWS its runner, so `/clear`
 * re-points a tab at a brand-new, unnamed chat. The tab has to be able to say so.
 */
export const UNTITLED_CHAT_LABEL = 'Untitled chat'

/** The label to show for a chat, falling back to the placeholder when it has no title. */
export function chatLabel(title: string | undefined | null): string {
  return title || UNTITLED_CHAT_LABEL
}

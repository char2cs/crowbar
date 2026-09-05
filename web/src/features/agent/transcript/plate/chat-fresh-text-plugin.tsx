import type { TText } from 'platejs'
import type { PlateEditor, PlateLeafProps } from 'platejs/react'
import { createPlatePlugin, PlateLeaf } from 'platejs/react'
import {
  CHAT_FRESH_DELAY_MARK,
  CHAT_FRESH_MARK,
} from '@/features/agent/transcript/plate/streaming-value-patch'

/**
 * Retires one faded-in leaf: unsets both its marks, which is all "done
 * fading" means here. Slate's own "merge adjacent leaves with identical
 * marks" normalization rule folds it back into plain text on its own —
 * nothing else has to track that the fade finished, or undo the split by
 * hand. Both marks, not just the fade flag: a leaf that kept its stagger
 * delay after settling would never look identical to an adjacent plain one
 * again, and never merge.
 *
 * Split out from the leaf component so it is callable (and testable)
 * without a real `animationend`, which jsdom cannot produce — it ships no
 * CSS engine, so no animation ever actually runs there to end.
 */
export function settleChatFreshText(editor: PlateEditor, text: TText): void {
  const path = editor.api.findPath(text)
  if (path) editor.tf.unsetNodes([CHAT_FRESH_MARK, CHAT_FRESH_DELAY_MARK], { at: path })
}

/**
 * Renders `CHAT_FRESH_MARK` — set only by streaming-value-patch.ts, on text a
 * full markdown parse already produced — as a fade from dim to full opacity
 * (transcript.css), delayed by `CHAT_FRESH_DELAY_MARK` so a whole chunk's
 * words cascade in instead of popping in together.
 *
 * Settles itself on the animation's real `animationend`, never a timer.
 */
function ChatFreshTextLeaf(props: PlateLeafProps) {
  const { editor, text, children } = props
  const delayMs = typeof text[CHAT_FRESH_DELAY_MARK] === 'number' ? text[CHAT_FRESH_DELAY_MARK] : 0
  return (
    <PlateLeaf {...props}>
      <span
        className="chat-fresh-text"
        style={{ animationDelay: `${delayMs}ms` }}
        onAnimationEnd={() => settleChatFreshText(editor, text)}
      >
        {children}
      </span>
    </PlateLeaf>
  )
}

export const ChatFreshTextPlugin = createPlatePlugin({
  key: CHAT_FRESH_MARK,
  node: { isLeaf: true },
}).withComponent(ChatFreshTextLeaf)

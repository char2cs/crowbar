import type { PlateLeafProps } from 'platejs/react'
import { createPlatePlugin, PlateLeaf } from 'platejs/react'
import {
  CHAT_FRESH_MARK,
  freshDecorations,
  freshLeafDelay,
  settleFreshGeneration,
} from '@/features/agent/transcript/plate/streaming-value-patch'

/**
 * Renders `CHAT_FRESH_MARK` — produced only by `streaming-value-patch.ts`, as
 * a DECORATION over text a full markdown parse already produced — as a fade
 * from invisible to full opacity (transcript.css), delayed by
 * `CHAT_FRESH_DELAY_MARK` so a whole chunk's words cascade in instead of
 * popping in together.
 *
 * Reads the delay off `props.leaf`, not `props.text`: `leaf` is the text node
 * WITH decorations applied, and the document's own text node never carries
 * these marks at all — that is the whole point of the decoration (see
 * `freshRuns` for the measured cost of having written them into the
 * document instead).
 *
 * Settles on the animation's real `animationend`, never a timer — and
 * settling is bookkeeping only (`settleFreshGeneration`), not an edit.
 */
function ChatFreshTextLeaf(props: PlateLeafProps) {
  const { editor, leaf, children } = props
  const delayMs = freshLeafDelay(leaf) ?? 0
  const generation = (leaf as unknown as Record<string, unknown>)[CHAT_FRESH_MARK]
  return (
    <PlateLeaf {...props}>
      <span
        className="chat-fresh-text"
        style={{ animationDelay: `${delayMs}ms` }}
        onAnimationEnd={() => {
          if (typeof generation === 'number') settleFreshGeneration(editor, generation)
        }}
      >
        {children}
      </span>
    </PlateLeaf>
  )
}

export const ChatFreshTextPlugin = createPlatePlugin({
  key: CHAT_FRESH_MARK,
  node: { isLeaf: true },
  decorate: ({ editor, entry }) => freshDecorations(editor, entry),
}).withComponent(ChatFreshTextLeaf)

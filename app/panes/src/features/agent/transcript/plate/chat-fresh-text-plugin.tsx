import { useMemo } from 'react'
import type { PlateLeafProps } from 'platejs/react'
import { createPlatePlugin, PlateLeaf } from 'platejs/react'
import {
  CHAT_FRESH_BORN_MARK,
  CHAT_FRESH_MARK,
  CHAT_FRESH_WORD_INDEX_MARK,
  CHAT_FRESH_WORD_TOTAL_MARK,
  freshDecorations,
  settleFreshGeneration,
  settleFreshWord,
} from '@/features/agent/transcript/plate/streaming-value-patch'
import { resumedFadeDelay } from '@/features/agent/transcript/plate/streaming-fade-delay'

/** Reads `props.leaf`, not `props.text`: the marks are decorations and never touch
 *  the document. Settles on the real `animationend`, never a timer. */
function ChatFreshTextLeaf(props: PlateLeafProps) {
  const { editor, leaf, children } = props
  const record = leaf as unknown as Record<string, unknown>
  const generation = record[CHAT_FRESH_MARK]
  const wordIndex = record[CHAT_FRESH_WORD_INDEX_MARK]
  const totalWords = record[CHAT_FRESH_WORD_TOTAL_MARK]
  const bornAt = record[CHAT_FRESH_BORN_MARK]
  // eslint-disable-next-line react-hooks/exhaustive-deps -- once per (span, run): rewriting animation-delay mid-animation jumps it
  const delayMs = useMemo(() => resumedFadeDelay(leaf) ?? 0, [generation, wordIndex, bornAt])
  return (
    <PlateLeaf {...props}>
      <span
        className="chat-fresh-text"
        style={{ animationDelay: `${delayMs}ms` }}
        onAnimationEnd={() => {
          if (typeof generation !== 'number') return
          // A per-word split retires the generation only once every word has
          // reported; an unsplit span settles directly.
          if (typeof wordIndex === 'number' && typeof totalWords === 'number') {
            settleFreshWord(editor, generation, wordIndex, totalWords)
          } else {
            settleFreshGeneration(editor, generation)
          }
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

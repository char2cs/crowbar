import { useMemo } from 'react'
import type { PlateLeafProps } from 'platejs/react'
import { createPlatePlugin, PlateLeaf } from 'platejs/react'
import {
  CHAT_FRESH_BORN_MARK,
  CHAT_FRESH_MARK,
  CHAT_FRESH_WORD_INDEX_MARK,
  CHAT_FRESH_WORD_TOTAL_MARK,
  freshDecorations,
  resumedFadeDelay,
  settleFreshGeneration,
  settleFreshWord,
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
  const record = leaf as unknown as Record<string, unknown>
  const generation = record[CHAT_FRESH_MARK]
  const wordIndex = record[CHAT_FRESH_WORD_INDEX_MARK]
  const totalWords = record[CHAT_FRESH_WORD_TOTAL_MARK]
  const bornAt = record[CHAT_FRESH_BORN_MARK]
  // ONCE per (span, run) — deps are all constants of the run, so a re-render
  // reuses the delay this span mounted with and only a genuine remount, or
  // this span being reused for a DIFFERENT run, recomputes it.
  //
  // Reading the clock on every render instead is what made settled text
  // repaint mid-sentence: `animation-delay` is relative to when the animation
  // started on the element, so rewriting it under a running animation jumps
  // that animation's current time. It also double-counts — the animation is
  // already advancing on its own — so the fade ran at roughly twice speed.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const delayMs = useMemo(() => resumedFadeDelay(leaf) ?? 0, [generation, wordIndex, bornAt])
  return (
    <PlateLeaf {...props}>
      <span
        className="chat-fresh-text"
        style={{ animationDelay: `${delayMs}ms` }}
        onAnimationEnd={() => {
          if (typeof generation !== 'number') return
          // Present only on a per-word split (see CHAT_FRESH_WORD_INDEX_MARK)
          // — this word's own end, not the whole chunk's, is what just
          // happened, so the generation only retires once every word sharing
          // it has reported. A capped run's single, unsplit span carries
          // neither field and settles the old way, directly: one span, one
          // fade, nothing to wait on.
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

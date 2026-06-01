import type { CSSProperties } from 'react'
import type { MarkdownTurn } from '../../types'
import { TurnMarkdown } from './turn-markdown'
// Import block registrations so they self-register into the registry.
import './blocks/mermaid-block'
import './blocks/excalidraw-block'

interface MarkdownHistoryProps {
  turns: MarkdownTurn[]
  onWidgetChange?: (widgetId: string, payload: unknown) => void
}

function metaLabel(turn: MarkdownTurn): string {
  const time = turn.timestamp
    ? new Date(turn.timestamp).toLocaleTimeString([], {
        hour: 'numeric',
        minute: '2-digit',
      })
    : ''
  // User turns: time only (the tint band already marks them as yours). Agent
  // turns lead with the model so it's clear who produced the answer.
  if (turn.role === 'agent') {
    const who = turn.model || turn.authorName
    return [who, time].filter(Boolean).join(' · ')
  }
  return time
}

// 3-column grid: [left margin] [centered ≤680 content] [right margin].
// The metadata sits in the left margin, right-justified against the content,
// and is `position: sticky` — so the browser pins it at the top of the viewport
// while its turn is scrolled through, then the next turn's label takes over.
// Native sticky, scoped per <article>: no overlay, no scroll JS.
const articleStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'minmax(48px, 1fr) minmax(0, 680px) minmax(48px, 1fr)',
  padding: '8px 0',
}

const metaStyle: CSSProperties = {
  gridColumn: 1,
  justifySelf: 'end',
  alignSelf: 'start',
  position: 'sticky',
  top: '10px',
  paddingRight: '16px',
  fontSize: '10px',
  lineHeight: '26px',
  whiteSpace: 'nowrap',
  fontVariantNumeric: 'tabular-nums',
}

export function MarkdownHistory({ turns, onWidgetChange }: MarkdownHistoryProps) {
  return (
    <div className="pt-10 pb-6">
      {turns.map((turn) => (
          <article
            key={turn.id}
            style={{
              ...articleStyle,
              background:
                turn.role === 'user'
                  ? 'color-mix(in srgb, var(--primary) 5%, transparent)'
                  : undefined,
            }}
          >
            {/* Hidden when the pane is too narrow to hold the label in the margin. */}
            <header
              className="text-muted-foreground @max-[880px]:hidden"
              style={metaStyle}
            >
              {metaLabel(turn)}
            </header>
            <div style={{ gridColumn: 2, minWidth: 0 }}>
              <TurnMarkdown
                content={turn.content}
                widgets={turn.widgets}
                streaming={turn.streaming}
                onWidgetChange={onWidgetChange}
              />
            </div>
          </article>
        ))}
    </div>
  )
}

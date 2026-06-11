import type { CSSProperties } from 'react'
import type { MarkdownTurn, TurnRole } from '../../types'
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

// Per-turn column rails: primary for the user's turns, foreground for the
// agent's. Low-opacity so they read as a subtle accent, not a hard border.
function railColor(role: TurnRole): string {
  return role === 'user'
    ? 'color-mix(in srgb, var(--primary) 35%, transparent)'
    : 'color-mix(in srgb, var(--foreground) 15%, transparent)'
}

// Left/right edge of the centered text column (same as the input's).
const COLUMN_EDGE = 'max(48px, calc((100% - 680px) / 2))'

// 3-column grid: [left margin] [centered ≤680 content] [right margin].
// The metadata sits in the left margin, right-justified against the content,
// and is `position: sticky` — so the browser pins it at the top of the viewport
// while its turn is scrolled through, then the next turn's label takes over.
// Native sticky, scoped per <article>: no overlay, no scroll JS.
const articleStyle: CSSProperties = {
  position: 'relative',
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
    <div className="pt-10">
      {turns.map((turn) => (
        <article
          key={turn.id}
          style={{
            ...articleStyle,
            // Same tint as the input/composer (markdown-chat-view).
            background:
              turn.role === 'user'
                ? 'color-mix(in srgb, var(--primary) 10%, transparent)'
                : undefined,
          }}
        >
          {/* Full-height column rails (span the tint and touch the adjacent
                turn's rails); colored by role. */}
          <div
            aria-hidden
            className="pointer-events-none absolute inset-y-0 w-px"
            style={{ left: COLUMN_EDGE, background: railColor(turn.role) }}
          />
          <div
            aria-hidden
            className="pointer-events-none absolute inset-y-0 w-px"
            style={{ right: COLUMN_EDGE, background: railColor(turn.role) }}
          />
          {/* Hidden when the pane is too narrow to hold the label in the margin. */}
          <header className="text-muted-foreground @max-[880px]:hidden" style={metaStyle}>
            {metaLabel(turn)}
          </header>
          <div
            style={{
              gridColumn: 2,
              minWidth: 0,
              paddingLeft: '24px',
              paddingRight: '24px',
            }}
          >
            <TurnMarkdown
              content={turn.content}
              widgets={turn.widgets}
              streaming={turn.streaming}
              onWidgetChange={onWidgetChange}
            />
            {turn.error && (
              <div
                role="alert"
                className="my-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
              >
                {turn.error}
              </div>
            )}
          </div>
        </article>
      ))}
    </div>
  )
}

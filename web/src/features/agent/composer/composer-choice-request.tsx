import type { CodeLine, RequestView } from '@/features/agent/composer/lib/ask-request'
import { foldCode, showAllLabel } from '@/features/agent/composer/lib/ask-request'
import { Visible } from '@/features/agent/composer/visible-text'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { McpServerBadge } from '@/features/agent/transcript/rows/mcp-server-badge'

const TONE: Record<NonNullable<CodeLine['tone']>, string> = { del: '- ', add: '+ ' }

/** What the permission would let run, by the kind of call it gates. */
export function ChoiceRequest({
  view,
  expanded,
  onExpand,
}: {
  view: RequestView
  expanded: boolean
  onExpand: () => void
}) {
  const folded = foldCode(view.code)
  const shown = expanded ? view.code : folded.lines
  const more = !expanded && (folded.moreLines > 0 || folded.moreChars > 0)
  return (
    <div className="choice-request" data-testid="agent-choice-request">
      {(view.server || view.head) && (
        <span className="choice-request-head">
          {view.server && (
            <>
              <McpServerBadge server={view.server} />
              {' · '}
            </>
          )}
          {view.head && <Visible text={view.head} />}
        </span>
      )}
      {view.code.length > 0 && (
        // Unfolded it scrolls inside its own bound, so the answers stay in reach.
        <pre
          className="choice-request-code"
          data-expanded={expanded || undefined}
          tabIndex={expanded ? 0 : undefined}
        >
          {shown.map((line, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- lines are positional and never reorder
            <span key={i} className={line.tone ? `choice-diff-${line.tone}` : undefined}>
              {line.tone && TONE[line.tone]}
              <Visible text={line.text} />
              {line.cut && <span className="choice-request-cut">…</span>}
              {'\n'}
            </span>
          ))}
        </pre>
      )}
      {view.markdown && (
        // It scrolls inside its own bound, so the answers stay in reach.
        <div className="choice-request-markdown" tabIndex={0}>
          <MarkdownMessageStatic variant="chat">{view.markdown}</MarkdownMessageStatic>
        </div>
      )}
      {more && (
        <button type="button" className="choice-request-more" onClick={onExpand}>
          {showAllLabel(view.code, folded)}
        </button>
      )}
      {view.note && (
        <span className="choice-request-note">
          <Visible text={view.note} />
        </span>
      )}
    </div>
  )
}

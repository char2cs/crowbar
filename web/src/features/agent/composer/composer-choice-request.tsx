import type { CodeLine, RequestView } from '@/features/agent/composer/lib/ask-request'
import { SHOWN_LINES, visibleParts } from '@/features/agent/composer/lib/ask-request'
import { McpServerBadge } from '@/features/agent/transcript/rows/mcp-server-badge'

function Visible({ text }: { text: string }) {
  return visibleParts(text).map((part, i) =>
    part.escaped ? (
      // react-doctor-disable-next-line no-array-index-as-key -- parts of one fixed string, never reordered
      <span key={i} className="choice-escape" title="A character that does not draw">
        {part.text}
      </span>
    ) : (
      part.text
    ),
  )
}

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
  const shown = expanded ? view.code : view.code.slice(0, SHOWN_LINES)
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
        <pre className="choice-request-code">
          {shown.map((line, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- lines are positional and never reorder
            <span key={i} className={line.tone ? `choice-diff-${line.tone}` : undefined}>
              {line.tone && TONE[line.tone]}
              <Visible text={line.text} />
              {'\n'}
            </span>
          ))}
        </pre>
      )}
      {shown.length < view.code.length && (
        <button type="button" className="choice-request-more" onClick={onExpand}>
          Show all {view.code.length} lines
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

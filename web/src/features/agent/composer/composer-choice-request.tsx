import { requestBody, type AskRequest } from '@/features/agent/composer/lib/ask-request'
import { McpServerBadge } from '@/features/agent/transcript/rows/mcp-server-badge'

/** What the permission would let run, by the kind of call it gates. */
export function ChoiceRequest({ request }: { request: AskRequest }) {
  const body = requestBody(request)
  switch (body.kind) {
    case 'truncated':
      return (
        <div className="choice-request" data-testid="agent-choice-request">
          <pre className="choice-request-code">{body.text}</pre>
          <span className="choice-request-note">Input truncated</span>
        </div>
      )
    case 'mcp':
      return (
        <div className="choice-request" data-testid="agent-choice-request">
          <span className="choice-request-head">
            <McpServerBadge server={body.server} /> · {body.tool}
          </span>
          {body.args.length > 0 && (
            <pre className="choice-request-code">{body.args.join('\n')}</pre>
          )}
        </div>
      )
    case 'command':
      return (
        <div className="choice-request" data-testid="agent-choice-request">
          <pre className="choice-request-code">{body.command}</pre>
          {body.description && <span className="choice-request-note">{body.description}</span>}
        </div>
      )
    case 'edit':
      return (
        <div className="choice-request" data-testid="agent-choice-request">
          {body.path && <span className="choice-request-head">{body.path}</span>}
          <pre className="choice-request-code">
            {body.removed.map((line, i) => (
              // react-doctor-disable-next-line no-array-index-as-key -- diff lines are positional and never reorder
              <span key={`-${i}`} className="choice-diff-del">{`- ${line}\n`}</span>
            ))}
            {body.added.map((line, i) => (
              // react-doctor-disable-next-line no-array-index-as-key -- diff lines are positional and never reorder
              <span key={`+${i}`} className="choice-diff-add">{`+ ${line}\n`}</span>
            ))}
          </pre>
        </div>
      )
    default:
      return null
  }
}

import type { ReactNode } from 'react'
import type { CodeLine, RequestView } from '@/features/agent/composer/lib/ask-request'
import { foldCode, showAllLabel } from '@/features/agent/composer/lib/ask-request'
import { Visible } from '@/features/agent/composer/visible-text'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

const SIGN: Record<NonNullable<CodeLine['tone']>, string> = { del: '−', add: '+', ctx: '' }

const countTone = (code: CodeLine[], tone: CodeLine['tone']) =>
  code.filter((line) => line.tone === tone).length

/** What a permission is about, under its title: an MCP call's server and tool, an edit's path
 *  and how many lines it adds and removes. Null for a command, which is its own body. */
export function ChoiceRequestHead({ view }: { view: RequestView }) {
  if (!view.server && !view.head) return null
  const added = countTone(view.code, 'add')
  const removed = countTone(view.code, 'del')
  return (
    <span className="rq-sub" data-testid="agent-choice-detail">
      <span className="rq-mono">
        {view.server && (
          <>
            <Visible text={view.server} />
            {' · '}
          </>
        )}
        {view.head && <Visible text={view.head} />}
      </span>
      {added + removed > 0 && (
        <>
          {' '}
          <span className="rq-add rq-mono">+{added}</span>{' '}
          <span className="rq-del rq-mono">−{removed}</span>
        </>
      )}
    </span>
  )
}

/** An MCP call's arguments as the labelled rows they are: a name opens each, its lines follow. */
function argRows(code: CodeLine[]): { key: string; lines: CodeLine[] }[] {
  const rows: { key: string; lines: CodeLine[] }[] = []
  for (const line of code) {
    if (line.key !== undefined || rows.length === 0) rows.push({ key: line.key ?? '', lines: [] })
    rows[rows.length - 1]!.lines.push(line)
  }
  return rows
}

function Lines({ lines }: { lines: CodeLine[] }) {
  return lines.map((line, i) => (
    // react-doctor-disable-next-line no-array-index-as-key -- lines are positional and never reorder
    <span key={i}>
      {i > 0 && '\n'}
      <Visible text={line.text} />
      {line.cut && <span className="choice-request-cut">…</span>}
    </span>
  ))
}

/** The operators of a command are dimmed, as the board draws them. */
function Command({ line }: { line: CodeLine }) {
  return line.text.split(/( && | \|\| )/).map((part, i) =>
    i % 2 === 1 ? (
      // react-doctor-disable-next-line no-array-index-as-key -- parts of one fixed string, never reorder
      <span key={i} className="rq-op">
        {part}
      </span>
    ) : (
      // react-doctor-disable-next-line no-array-index-as-key -- parts of one fixed string, never reorder
      <Visible key={i} text={part} />
    ),
  )
}

/** A diff line's text, the span that changed in a `<mark>`. */
function Marked({ line }: { line: CodeLine }) {
  if (!line.mark) return <Visible text={line.text} />
  const [from, to] = line.mark
  return (
    <>
      <Visible text={line.text.slice(0, from)} />
      <mark>
        <Visible text={line.text.slice(from, to)} />
      </mark>
      <Visible text={line.text.slice(to)} />
    </>
  )
}

function CodeBody({
  shown,
  view,
  expanded,
}: {
  shown: CodeLine[]
  view: RequestView
  expanded: boolean
}): ReactNode {
  if (view.server) {
    return (
      <div className="rq-args" data-expanded={expanded || undefined}>
        {argRows(shown).map((row, i) => (
          // react-doctor-disable-next-line no-array-index-as-key -- rows are positional and never reorder
          <div key={i} className="rq-arg">
            <span className="rq-arg-key">{row.key}</span>
            <span>
              <Lines lines={row.lines} />
            </span>
          </div>
        ))}
      </div>
    )
  }
  if (shown.some((line) => line.tone)) {
    return (
      <div
        className="rq-diff"
        data-expanded={expanded || undefined}
        tabIndex={expanded ? 0 : undefined}
      >
        {shown.map((line, i) => (
          // react-doctor-disable-next-line no-array-index-as-key -- lines are positional and never reorder
          <div key={i} className="rq-ln" data-tone={line.tone}>
            <span className="rq-ln-n" aria-hidden="true">
              {line.n}
            </span>
            <span className="rq-ln-sign" aria-hidden="true">
              {line.tone && SIGN[line.tone]}
            </span>
            <span className="rq-ln-text">
              <Marked line={line} />
              {line.cut && <span className="choice-request-cut">…</span>}
            </span>
          </div>
        ))}
      </div>
    )
  }
  return (
    // Unfolded it scrolls inside its own bound, so the answers stay in reach.
    <pre
      className="choice-request-code"
      data-shell={view.shell || undefined}
      data-expanded={expanded || undefined}
      tabIndex={expanded ? 0 : undefined}
    >
      <span>
        {shown.map((line, i) => (
          // react-doctor-disable-next-line no-array-index-as-key -- lines are positional and never reorder
          <span key={i}>
            {i > 0 && '\n'}
            {view.shell ? <Command line={line} /> : <Visible text={line.text} />}
            {line.cut && <span className="choice-request-cut">…</span>}
          </span>
        ))}
        {'\n'}
      </span>
    </pre>
  )
}

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
      {view.code.length > 0 && <CodeBody shown={shown} view={view} expanded={expanded} />}
      {view.markdown && (
        // It scrolls inside its own bound, so the answers stay in reach.
        <div className="choice-request-markdown">
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

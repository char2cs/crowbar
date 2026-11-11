import { memo, useCallback, useState } from 'react'
import {
  CaretRightIcon,
  CheckIcon,
  CircleDashedIcon,
  ProhibitIcon,
  StopIcon,
  XIcon,
} from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TurnDiffPreview } from '@/features/agent/activity/turn-diff-preview'
import { useLiveText } from '@/features/agent/hooks/use-chat-feed'
import { describeResolvedChoice } from '@/features/agent/composer/lib/ask-labels'
import {
  FetchIcon,
  FileIcon,
  PencilIcon,
  SearchIcon,
  SubagentIcon,
  TerminalIcon,
  ToolIcon,
} from '@/features/agent/shared/agent-icons'
import type { AskItem, ToolItem } from '@/features/agent/stores/lib/transcript-items'
import {
  diffStat,
  formatDuration,
  labelText,
  toolLabel,
} from '@/features/agent/transcript/lib/tool-label'
import { McpServerBadge } from '@/features/agent/transcript/rows/mcp-server-badge'
import { ToolPayload } from '@/features/agent/transcript/rows/tool-payload'
import { plain } from '@/features/agent/transcript/lib/payload-text'

const FAMILY_ICONS: Partial<Record<ToolItem['family'], typeof ToolIcon>> = {
  read: FileIcon,
  edit: PencilIcon,
  execute: TerminalIcon,
  search: SearchIcon,
  fetch: FetchIcon,
  subagent: SubagentIcon,
}

/** A call's outcome when it is not simply "went well": each reads differently at a glance. */
interface Outcome {
  kind: 'failed' | 'denied' | 'stopped' | 'unavailable' | 'awaiting'
  text: string
  Glyph: Icon
}

const OUTCOMES: Partial<Record<ToolItem['status'], Outcome>> = {
  error: { kind: 'failed', text: 'Failed', Glyph: XIcon },
  declined: { kind: 'denied', text: 'Denied', Glyph: ProhibitIcon },
  interrupted: { kind: 'stopped', text: 'Stopped', Glyph: StopIcon },
  abandoned: { kind: 'unavailable', text: 'Result unavailable', Glyph: CircleDashedIcon },
}

/** How an ask was decided, as the look its pill takes: a yes is quiet, a no is the
 *  warning tone, an answer that never came is dashed. */
function noteKind(note: string): string {
  if (note.startsWith('Denied')) return 'denied'
  if (note === 'Timed out' || note === 'Not answered') return 'unavailable'
  return 'allowed'
}

const AWAITING: Outcome = {
  kind: 'awaiting',
  text: 'Needs your answer',
  Glyph: CircleDashedIcon,
}

/** Running spins, a call that went well is checked, every other outcome has its own glyph. */
function StatusGlyph({ status, outcome }: { status: ToolItem['status']; outcome?: Outcome }) {
  if (outcome) {
    const { Glyph } = outcome
    return (
      <Glyph
        size={12}
        weight={outcome.kind === 'stopped' ? 'fill' : 'regular'}
        className="tool-glyph"
      />
    )
  }
  if (status === 'running') return <FlickerSpinner className="size-3" />
  return status === 'ok' ? <CheckIcon size={12} className="tool-glyph" /> : null
}

function Label({ item }: { item: ToolItem }) {
  const { verb, subject } = toolLabel(item)
  const live = item.status === 'running' ? '' : undefined
  if (item.family === 'mcp') {
    return (
      <span className="tool-call-label" data-testid="agent-tool-label">
        <McpServerBadge server={verb} />
        <span className="tool-call-verb" data-live={live}>
          {' · '}
          {subject?.display}
        </span>
      </span>
    )
  }
  return (
    <span className="tool-call-label" data-testid="agent-tool-label">
      <span className="tool-call-verb" data-live={live}>
        {verb}
      </span>
      {subject && (
        <span className="tool-call-subject" title={subject.full}>
          {subject.display}
        </span>
      )}
    </span>
  )
}

function ToolRowBody({ chatId, item }: { chatId: string; item: ToolItem }) {
  const [open, setOpen] = useState(false)
  const { body, status, family } = item
  const liveDiff = useLiveText(chatId, item.toolId, 'diff')
  const patch = family === 'edit' ? body.diff || liveDiff || '' : ''
  const stat = patch ? diffStat(patch) : null
  const expandable = Boolean(body.inputRef || body.resultRef || body.output || body.error || patch)
  const Icon = FAMILY_ICONS[family] ?? ToolIcon
  const outcome = item.askId !== undefined ? AWAITING : OUTCOMES[status]
  // What a call cost is on hover, never on the line.
  const cost = [
    stat && `+${stat.added} −${stat.removed}`,
    body.durationMs !== undefined && formatDuration(body.durationMs),
  ]
    .filter(Boolean)
    .join(' · ')
  // The call waiting on the person is brought into view while it waits.
  const awaiting = item.askId !== undefined
  const reveal = useCallback(
    (el: HTMLLIElement | null) => {
      if (awaiting) el?.scrollIntoView?.({ block: 'nearest' })
    },
    [awaiting],
  )
  const summary = (
    <span className="tool-call-summary" title={cost || undefined}>
      <StatusGlyph status={status} outcome={outcome} />
      <span className="kind" data-tool-kind={body.kind}>
        <Icon size={12} />
      </span>
      <Label item={item} />
      {/* How its ask was decided says why it did not run; the outcome would repeat it. */}
      {item.askNote ? (
        <span
          className="tool-call-note"
          data-outcome={noteKind(item.askNote)}
          data-testid="agent-tool-ask-note"
        >
          {item.askNote}
        </span>
      ) : (
        outcome && (
          <span className="tool-call-state" data-outcome={outcome.kind}>
            {outcome.text}
          </span>
        )
      )}
    </span>
  )
  return (
    <article
      className="row activity-tool-row"
      data-row-id={item.id}
      data-testid="agent-activity-tool"
      data-tool-id={item.toolId}
      data-family={family}
      aria-label={labelText(item)}
    >
      <div className="assistant">
        <ul className="tools">
          <li data-status={awaiting ? 'awaiting_approval' : status} ref={reveal}>
            <div className="tool-row">
              {summary}
              {expandable && (
                <button
                  type="button"
                  className="tool-row-toggle"
                  aria-label="Show tool details"
                  aria-expanded={open}
                  onClick={() => setOpen((o) => !o)}
                >
                  <CaretRightIcon size={12} />
                </button>
              )}
            </div>
            {open && patch && (
              <div className="turn-diff tool-turn-diff" data-testid="agent-tool-diff">
                <TurnDiffPreview diff={patch} turnId={item.toolId} target={body.target} />
              </div>
            )}
            {open && (body.output || body.error) && (
              <pre className="tool-call-output" data-testid="agent-tool-short-output">
                {plain(body.output || body.error || '')}
              </pre>
            )}
            {/* The blobs are read only once someone asks for them. */}
            {open && (body.inputRef || body.resultRef) && (
              <ToolPayload chatId={chatId} inputRef={body.inputRef} resultRef={body.resultRef} />
            )}
          </li>
        </ul>
      </div>
    </article>
  )
}

export const ToolRow = memo(ToolRowBody)

/** A resolved permission, question or elicitation: the record of what was
 *  decided. A pending one is the composer's, so nothing is drawn for it here. */
function AskRowBody({ item }: { item: AskItem }) {
  if (item.choice.pending) return null
  return (
    <article className="row" data-row-id={item.id}>
      <div className="assistant">
        <ul className="tools" data-testid="agent-turn-choices">
          <li data-resolution={item.choice.resolution}>
            <span>{describeResolvedChoice(item.choice)}</span>
          </li>
        </ul>
      </div>
    </article>
  )
}

export const AskRow = memo(AskRowBody)

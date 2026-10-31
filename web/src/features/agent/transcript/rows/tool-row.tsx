import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TurnDiffPreview } from '@/features/agent/activity/turn-diff-preview'
import { useLiveText } from '@/features/agent/hooks/use-chat-feed'
import { describeResolvedChoice } from '@/features/agent/lib/agent-activity'
import {
  CheckIcon,
  CloseIcon,
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
  commandProgramName,
  diffStat,
  formatDuration,
  labelText,
  tailOf,
  toolLabel,
} from '@/features/agent/transcript/lib/tool-label'
import { McpServerBadge } from '@/features/agent/transcript/rows/mcp-server-badge'
import { ToolPayload } from '@/features/agent/transcript/rows/tool-payload'

/** A running tool's output is a progress signal, not a log: one line's worth. */
const TOOL_OUTPUT_LIMIT = 120

/** An edit this small reads at a glance, so its diff starts open. */
const OPEN_DIFF_LINES = 12

const FAMILY_ICONS: Partial<Record<ToolItem['family'], typeof ToolIcon>> = {
  read: FileIcon,
  edit: PencilIcon,
  execute: TerminalIcon,
  search: SearchIcon,
  fetch: FetchIcon,
  subagent: SubagentIcon,
}

/** What a call that never finished says in place of a result. */
const STATE_TEXT: Partial<Record<ToolItem['status'], string>> = {
  declined: 'Denied',
  interrupted: 'Stopped',
  abandoned: 'Result unavailable',
}

function StatusGlyph({ status }: { status: ToolItem['status'] }) {
  if (status === 'running') return <FlickerSpinner className="size-3" />
  if (status === 'ok') return <CheckIcon size={12} className="tool-glyph" />
  return <CloseIcon size={12} className="tool-glyph" />
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

/** The command itself, when the label named it by something else. */
function commandLine(item: ToolItem): string {
  const target = item.body.target?.trim() ?? ''
  if (item.family !== 'execute' || !target) return ''
  const shown = item.body.title || commandProgramName(target)
  return shown === target ? '' : (target.split('\n')[0] ?? '')
}

function ToolRowBody({ chatId, item }: { chatId: string; item: ToolItem }) {
  const [toggled, setToggled] = useState<boolean | null>(null)
  const { body, status, family } = item
  const running = status === 'running'
  const output = useLiveText(chatId, item.toolId, 'tool_output')
  const liveDiff = useLiveText(chatId, item.toolId, 'diff')
  const patch = family === 'edit' ? body.diff || liveDiff || '' : ''
  const stat = patch ? diffStat(patch) : null
  const showDiff = toggled ?? (stat !== null && stat.added + stat.removed <= OPEN_DIFF_LINES)
  const expandable = Boolean(body.inputRef || body.resultRef || patch)
  const Icon = FAMILY_ICONS[family] ?? ToolIcon
  const errorLine = status === 'error' ? body.error?.split('\n')[0] : undefined
  const exit =
    body.exitCode && (body.resultRef || output) ? (
      <span className="tool-call-exit">exit {body.exitCode}</span>
    ) : null
  const command = commandLine(item)
  const summary = (
    <span className="tool-call-summary">
      <StatusGlyph status={status} />
      <span className="kind" data-tool-kind={body.kind}>
        <Icon size={12} />
      </span>
      <Label item={item} />
      {STATE_TEXT[status] && <span className="tool-call-state">{STATE_TEXT[status]}</span>}
      {stat && (
        <span className="tool-call-stat">
          +{stat.added} −{stat.removed}
        </span>
      )}
      {exit}
      {errorLine && <span className="err">{errorLine}</span>}
      {command && <span className="tool-call-command">$ {command}</span>}
      {body.durationMs !== undefined && (
        <span className="tool-call-duration">{formatDuration(body.durationMs)}</span>
      )}
      {running && output && (
        <span className="out" data-testid="agent-tool-output">
          {tailOf(output, TOOL_OUTPUT_LIMIT)}
        </span>
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
          <li data-status={status}>
            <div className="tool-row">
              {summary}
              {expandable && (
                <button
                  type="button"
                  className="tool-row-toggle"
                  aria-label="Show tool details"
                  aria-expanded={toggled ?? showDiff}
                  onClick={() => setToggled(!(toggled ?? showDiff))}
                >
                  <CaretRightIcon size={12} />
                </button>
              )}
            </div>
            {patch && showDiff && (
              <div className="turn-diff tool-turn-diff" data-testid="agent-tool-diff">
                <TurnDiffPreview diff={patch} turnId={item.toolId} />
              </div>
            )}
            {/* The blobs are read only once someone asks for them. */}
            {toggled && (body.inputRef || body.resultRef) && (
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

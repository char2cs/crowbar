import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import type { ToolCallBody } from '@/features/agent/api/chat-rows'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TurnDiffPreview } from '@/features/agent/activity/turn-diff-preview'
import { useLiveText } from '@/features/agent/hooks/use-chat-feed'
import { describeResolvedChoice, formatDuration, tailOf } from '@/features/agent/lib/agent-activity'
import {
  FetchIcon,
  FileIcon,
  PencilIcon,
  SearchIcon,
  TerminalIcon,
  ToolIcon,
} from '@/features/agent/shared/agent-icons'
import type { AskItem, ToolItem } from '@/features/agent/stores/lib/transcript-items'
import { ToolPayload } from '@/features/agent/transcript/rows/tool-payload'

/** A running tool's output is a progress signal, not a log: one line's worth. */
const TOOL_OUTPUT_LIMIT = 120

const KIND_ICONS: Record<string, typeof ToolIcon> = {
  read: FileIcon,
  edit: PencilIcon,
  execute: TerminalIcon,
  search: SearchIcon,
  fetch: FetchIcon,
}

const VERBS: Record<string, [string, string]> = {
  read: ['Reading', 'Read'],
  edit: ['Editing', 'Edited'],
  execute: ['Running', 'Ran'],
  search: ['Searching', 'Searched'],
  fetch: ['Fetching', 'Fetched'],
}

function humanizeToolName(name: string): string {
  const words = name
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .trim()
    .toLowerCase()
  return words ? `${words[0]?.toUpperCase()}${words.slice(1)}` : ''
}

function toolVerb(body: ToolCallBody, running: boolean): string {
  const verbs = body.kind ? VERBS[body.kind] : undefined
  if (verbs) return verbs[running ? 0 : 1]
  return humanizeToolName(body.name ?? '') || 'Tool'
}

/** The last three segments name a file; the full path stays in the tooltip. */
function compactPath(path: string): string {
  const normalized = path.replaceAll('\\', '/').replace(/\/+$/, '')
  if (!normalized.startsWith('/')) return normalized
  return normalized.split('/').filter(Boolean).slice(-3).join('/') || path
}

function toolSubject(body: ToolCallBody): { full: string; display: string } | undefined {
  const location = body.locations?.[0]
  if (location) {
    const line = location.line ? `:${location.line}` : ''
    const extra =
      body.locations && body.locations.length > 1 ? ` +${body.locations.length - 1}` : ''
    return {
      full: `${location.path}${line}${extra}`,
      display: `${compactPath(location.path)}${line}${extra}`,
    }
  }
  const target = body.target?.trim()
  if (!target) return undefined
  const pathLike =
    (body.kind === 'read' || body.kind === 'edit' || body.kind === 'search') &&
    target.startsWith('/') &&
    !target.includes('\n')
  return { full: target, display: pathLike ? compactPath(target) : target }
}

function ToolRowBody({ chatId, item }: { chatId: string; item: ToolItem }) {
  const [open, setOpen] = useState(false)
  const { body, status } = item
  const running = status === 'running'
  const output = useLiveText(chatId, item.toolId, 'tool_output')
  const liveDiff = useLiveText(chatId, item.toolId, 'diff')
  const subject = toolSubject(body)
  const Icon = body.kind ? (KIND_ICONS[body.kind] ?? ToolIcon) : null
  const patch = body.diff || liveDiff
  const expandable = Boolean(body.inputRef || body.resultRef)
  const summary = (
    <span className="tool-call-summary">
      {running && <FlickerSpinner className="size-3" />}
      {Icon && (
        <span className="kind" title={body.kind} data-tool-kind={body.kind}>
          <Icon size={12} />
        </span>
      )}
      <span className="tool-call-verb">{toolVerb(body, running)}</span>
      {subject && (
        <span
          className="tool-call-subject"
          data-tool-location={body.locations?.length ? '' : undefined}
          title={subject.full}
        >
          {subject.display}
        </span>
      )}
      {body.error && <span className="err">{body.error}</span>}
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
    >
      <div className="assistant">
        <ul className="tools">
          <li data-status={status}>
            {expandable ? (
              <div className="tool-row">
                {summary}
                <button
                  type="button"
                  className="tool-row-toggle"
                  aria-label="Show tool details"
                  aria-expanded={open}
                  onClick={() => setOpen((o) => !o)}
                >
                  <CaretRightIcon size={12} />
                </button>
              </div>
            ) : (
              summary
            )}
            {open && (
              <ToolPayload chatId={chatId} inputRef={body.inputRef} resultRef={body.resultRef} />
            )}
            {body.kind === 'edit' && patch && (
              <div className="turn-diff tool-turn-diff" data-testid="agent-tool-diff">
                <TurnDiffPreview diff={patch} turnId={item.toolId} />
              </div>
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

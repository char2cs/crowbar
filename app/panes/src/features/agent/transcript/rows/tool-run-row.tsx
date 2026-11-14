import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import { useShallow } from 'zustand/react/shallow'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { ToolItem } from '@/features/agent/stores/lib/transcript-items'
import { liveLabel } from '@/features/agent/transcript/lib/tool-label'
import { runSentence } from '@/features/agent/transcript/lib/tool-runs'
import { ToolRow } from '@/features/agent/transcript/rows/tool-row'

const standsAlone = (item: ToolItem) =>
  item.askId !== undefined ||
  item.askNote !== undefined ||
  (item.status !== 'running' && item.status !== 'ok')

/** Consecutive read-only calls as one line: a ticker on the newest while any
 *  runs, a sentence once they are done; expanding lists each call's own row. */
function FoldedRun({ chatId, id, tools }: { chatId: string; id: string; tools: ToolItem[] }) {
  const [open, setOpen] = useState(false)
  const running = tools.filter((item) => item.status === 'running')
  const live = running.length > 0
  const newest = running.at(-1)
  return (
    <article className="row activity-tool-row" data-row-id={id}>
      <div className="assistant">
        <ul className="tools">
          <li data-status={live ? 'running' : 'ok'} data-testid="agent-tool-run">
            <div className="tool-row">
              <span className="tool-call-summary">
                {live && <FlickerSpinner className="size-3" />}
                <span className="tool-call-verb" data-live={live ? '' : undefined}>
                  {runSentence(tools, live)}
                </span>
                {newest && (
                  <span className="tool-call-subject" data-testid="agent-tool-run-ticker">
                    {liveLabel(newest)}
                  </span>
                )}
              </span>
              <button
                type="button"
                className="tool-row-toggle"
                aria-label="Show the run’s tool calls"
                aria-expanded={open}
                onClick={() => setOpen((o) => !o)}
              >
                <CaretRightIcon size={12} />
              </button>
            </div>
            {open && (
              <div className="tool-run-members">
                {tools.map((item) => (
                  <ToolRow key={item.id} chatId={chatId} item={item} />
                ))}
              </div>
            )}
          </li>
        </ul>
      </div>
    </article>
  )
}

/** A run cut around its calls that stand alone, in order; the rest stay folded. */
function segments(tools: ToolItem[]): ToolItem[][] {
  const out: ToolItem[][] = []
  let folded: ToolItem[] = []
  for (const item of tools) {
    if (!standsAlone(item)) {
      folded.push(item)
      continue
    }
    if (folded.length > 0) out.push(folded)
    out.push([item])
    folded = []
  }
  if (folded.length > 0) out.push(folded)
  return out
}

function ToolRunRowBody({
  chatId,
  id,
  members,
}: {
  chatId: string
  id: string
  members: string[]
}) {
  const items = useChatStore(
    useShallow((s) =>
      members.map((member) => s.feeds[chatId]?.items.get(member) as ToolItem | undefined),
    ),
  )
  const tools = items.filter((item): item is ToolItem => item?.kind === 'tool')
  // A call that was asked about or did not succeed is a fact of its own: it is
  // drawn as its own row, and the calls around it fold on either side.
  return segments(tools).map((part, i) =>
    part.length === 1 ? (
      <ToolRow key={part[0].id} chatId={chatId} item={part[0]} />
    ) : (
      <FoldedRun
        key={part[0].id}
        chatId={chatId}
        id={i === 0 ? id : `g:${part[0].id}`}
        tools={part}
      />
    ),
  )
}

export const ToolRunRow = memo(ToolRunRowBody)

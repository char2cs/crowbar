import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import type { ReasoningItem } from '@/features/agent/stores/lib/transcript-items'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

/** What the model thought on its way to the reply, collapsed to how long it took. */
function ReasoningRowBody({ item }: { item: ReasoningItem }) {
  const [open, setOpen] = useState(false)
  const seconds = Math.round((Date.parse(item.endedAt) - Date.parse(item.startedAt)) / 1000)
  const label = seconds > 0 ? `Thought for ${seconds}s` : 'Thought'
  return (
    <article className="row reasoning-row" data-row-id={item.id}>
      <div className="assistant">
        <button
          type="button"
          className="reasoning-toggle"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          {label}
          <CaretRightIcon size={12} />
        </button>
        {open && (
          <MarkdownMessageStatic className="reasoning-text">{item.text}</MarkdownMessageStatic>
        )}
      </div>
    </article>
  )
}

export const ReasoningRow = memo(ReasoningRowBody)

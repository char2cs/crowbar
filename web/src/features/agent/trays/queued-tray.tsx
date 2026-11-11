import { XIcon } from '@phosphor-icons/react'
import { discardPrompt } from '@/features/agent/stores/chat-outbox'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'

/** Messages typed while the agent works, waiting for their turn: each can be taken back. */
export function QueuedTray({ chatId, items }: { chatId: string; items: OutboxItem[] }) {
  if (items.length === 0) return null
  return (
    <section className="tray-section" data-testid="agent-queued-tray">
      {items.length > 1 && (
        <p className="tray-head">
          <b>{items.length} queued</b>
        </p>
      )}
      <ul className="tray-list" data-flat={items.length === 1 ? '' : undefined}>
        {items.map((item) => (
          <li key={item.requestId}>
            {items.length === 1 && <span className="tray-label">Queued</span>}
            <span className="tray-text">{item.text}</span>
            <button
              type="button"
              className="tray-remove"
              aria-label="Remove queued message"
              title="Remove"
              onClick={() => discardPrompt(chatId, item.requestId)}
            >
              <XIcon size={13} />
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

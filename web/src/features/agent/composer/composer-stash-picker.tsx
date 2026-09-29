import { useEffect, useRef } from 'react'
import type { PromptStash } from '@/features/agent/composer/lib/prompt-stash-persistence'

interface ComposerStashPickerProps {
  items: PromptStash[]
  onSelect: (item: PromptStash) => void
  onClose: () => void
}

function summary(markdown: string): string {
  const firstContentLine = markdown
    .split('\n')
    .map((line) => line.trim())
    .find(Boolean)
  return (firstContentLine ?? 'Untitled draft').replace(/^#{1,6}\s+/, '').slice(0, 96)
}

function savedLabel(createdAt: string): string {
  const date = new Date(createdAt)
  return Number.isFinite(date.getTime())
    ? date.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })
    : 'Saved draft'
}

/** A small Crowbar-native menu shown only when an empty composer has choices. */
export function ComposerStashPicker({ items, onSelect, onClose }: ComposerStashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [])

  return (
    <div
      ref={listRef}
      className="stash-picker"
      role="menu"
      aria-label="Stashed prompts"
      onKeyDown={(event) => {
        if (event.key !== 'Escape') return
        event.preventDefault()
        onClose()
      }}
    >
      <div className="stash-picker-title">Restore a stashed prompt</div>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="menuitem"
          className="stash-picker-item"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onSelect(item)}
        >
          <span className="stash-picker-summary">{summary(item.markdown)}</span>
          <span className="stash-picker-time">{savedLabel(item.createdAt)}</span>
        </button>
      ))}
    </div>
  )
}

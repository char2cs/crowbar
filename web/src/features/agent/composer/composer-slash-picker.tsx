import { useEffect, useRef } from 'react'
import type { SlashCatalogItem } from '@/features/agent/api/agent-chat-reads'
import type { SlashCatalogState } from '@/features/agent/hooks/use-slash-catalog'
import { cn } from '@/lib/utils'

interface ComposerSlashPickerProps {
  state: SlashCatalogState
  items: SlashCatalogItem[]
  selected: number
  onSelect: (item: SlashCatalogItem) => void
}

/** No explanatory chrome: no probe reports a CLI's built-ins, so the catalogue is
 *  always incomplete, and an empty result must not claim the provider has no skills. */
export function ComposerSlashPicker({
  state,
  items,
  selected,
  onSelect,
}: ComposerSlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>('.opt.on')?.scrollIntoView({ block: 'nearest' })
  }, [selected])

  return (
    <div ref={listRef} className="slash" id="agent-skill-picker" role="listbox" aria-label="Skills">
      {state.state === 'error' && (
        <div className="state" role="alert">
          {state.unavailable ? 'No skill list from this provider.' : state.error.message}
        </div>
      )}
      {items.map((item, index) => (
        <button
          key={item.id}
          type="button"
          role="option"
          aria-selected={index === selected}
          className={cn('opt', index === selected && 'on')}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onSelect(item)}
        >
          <span className="t">{item.insertText.trim()}</span>
          {item.description && <span className="d">{item.description}</span>}
        </button>
      ))}
    </div>
  )
}

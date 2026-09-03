import { useEffect, useRef } from 'react'
import type { SlashCatalogItem } from '@/features/agent/api/agent-api'
import type { SlashCatalogState } from '@/features/agent/hooks/use-slash-catalog'
import { cn } from '@/lib/utils'

interface ComposerSlashPickerProps {
  state: SlashCatalogState
  items: SlashCatalogItem[]
  selected: number
  onSelect: (item: SlashCatalogItem) => void
}

/**
 * The skills a provider will answer to.
 *
 * One line per skill: the command in mono, its description beside it, both at
 * 13px. The command was 14px mono, which sets optically larger than the UI face
 * at the same size and made every row read as a heading.
 *
 * There is no explanatory chrome. The catalogue is INCOMPLETE BY DECLARATION on
 * every shipped provider — no probe reports a CLI's own built-ins — so a row
 * that is missing is normal, and a paragraph apologising for it on every
 * keystroke is noise. An empty result says nothing at all rather than claiming
 * the provider has no skills.
 */
export function ComposerSlashPicker({
  state,
  items,
  selected,
  onSelect,
}: ComposerSlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  // Keep the highlighted row in view as the arrow keys move it — the list
  // scrolls, the pointer never has to hunt for where it landed.
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

import { useEffect, useRef } from 'react'
import type { SlashCatalogState } from '@/features/agent/hooks/use-slash-catalog'
import { cn } from '@/lib/utils'

/** One completion the picker offers: a skill, or a workspace path after an `@`. */
export interface PickerItem {
  id: string
  insertText: string
  description: string
}

interface ComposerSlashPickerProps {
  /** The skill catalogue's state; a path listing has none to report. */
  state?: SlashCatalogState
  /** What the list is of, for its name. */
  label?: string
  items: PickerItem[]
  selected: number
  onSelect: (item: PickerItem) => void
}

/** No explanatory chrome: no probe reports a CLI's built-ins, so the catalogue is
 *  always incomplete, and an empty result must not claim the provider has no skills. */
export function ComposerSlashPicker({
  state,
  label = 'Skills',
  items,
  selected,
  onSelect,
}: ComposerSlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>('.opt.on')?.scrollIntoView({ block: 'nearest' })
  }, [selected])

  return (
    <div ref={listRef} className="slash" id="agent-skill-picker" role="listbox" aria-label={label}>
      {state?.state === 'error' && (
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

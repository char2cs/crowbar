import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

/** One completion the picker offers: a command or skill, or a workspace path after an `@`. */
export interface PickerItem {
  id: string
  insertText: string
  description: string
  /** Running it opens a dialog only the CLI's terminal shows. */
  terminal?: boolean
}

interface ComposerSlashPickerProps {
  /** What the list is of, for its name. */
  label?: string
  items: PickerItem[]
  selected: number
  onSelect: (item: PickerItem) => void
}

/** No explanatory chrome, and nothing at all over no match: the catalogue is partial by
 *  declaration, so an empty list must not claim the provider has no such command. */
export function ComposerSlashPicker({
  label = 'Commands',
  items,
  selected,
  onSelect,
}: ComposerSlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>('.opt.on')?.scrollIntoView({ block: 'nearest' })
  }, [selected])

  if (items.length === 0) return null
  return (
    <div ref={listRef} className="slash" id="agent-skill-picker" role="listbox" aria-label={label}>
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
          {item.terminal && <span className="h">Runs in the terminal</span>}
        </button>
      ))}
    </div>
  )
}

import { type CSSProperties, useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import {
  Autocomplete,
  AutocompleteEmpty,
  AutocompleteInput,
  AutocompleteItem,
  AutocompleteList,
  AutocompletePopup,
} from '@/components/ui/autocomplete'
import type { SlashCommand } from '../types'

// Re-export so existing importers keep working.
export type { SlashCommand }

interface SlashCommandPaletteProps {
  /** Provider-supplied commands. We don't own this list. */
  commands: SlashCommand[]
  onSelect: (command: SlashCommand) => void
  onClose: () => void
  anchorRect: DOMRect
}

export function SlashCommandPalette({
  commands,
  onSelect,
  onClose,
  anchorRect,
}: SlashCommandPaletteProps) {
  const [query, setQuery] = useState('')

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return commands
    return commands.filter(
      (c) =>
        c.label.toLowerCase().includes(q) ||
        c.description.toLowerCase().includes(q),
    )
  }, [commands, query])

  // Pin the palette's own search input to the editor caret; the Autocomplete
  // list floats from that input.
  const anchorStyle: CSSProperties = {
    position: 'fixed',
    top: anchorRect.top,
    left: anchorRect.left,
    transform: 'translateY(-100%)',
    zIndex: 50,
  }

  return (
    <Autocomplete
      items={filtered}
      mode="none"
      open
      value={query}
      onValueChange={setQuery}
      onOpenChange={(open) => { if (!open) onClose() }}
      autoHighlight="always"
      itemToStringValue={(c: SlashCommand) => c.label}
    >
      <div style={anchorStyle}>
        <AutocompleteInput
          autoFocus
          size="sm"
          startAddon={<Search />}
          placeholder="Search commands…"
          className="w-72"
        />
      </div>
      <AutocompletePopup side="top" align="start" sideOffset={8} className="w-72">
        <AutocompleteList>
          <AutocompleteEmpty>No commands</AutocompleteEmpty>
          {filtered.map((cmd) => (
            <AutocompleteItem
              key={cmd.id}
              value={cmd}
              onClick={() => onSelect(cmd)}
              className="items-start gap-2 py-1.5"
            >
              {cmd.icon && (
                <span className="mt-0.5 text-base leading-none">{cmd.icon}</span>
              )}
              <span className="flex min-w-0 flex-col">
                <span className="font-mono font-medium text-foreground">{cmd.label}</span>
                <span className="truncate text-xs text-muted-foreground">{cmd.description}</span>
              </span>
            </AutocompleteItem>
          ))}
        </AutocompleteList>
      </AutocompletePopup>
    </Autocomplete>
  )
}

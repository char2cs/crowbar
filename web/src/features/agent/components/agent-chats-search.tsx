import { MagnifyingGlass as Search } from '@phosphor-icons/react'
import { Input } from '@/components/ui/input'
import { SidebarHeader } from '@/components/ui/sidebar'

interface AgentChatsSearchProps {
  value: string
  onChange: (next: string) => void
}

/**
 * The Chats panel's filter.
 *
 * The FILE EXPLORER'S field, down to the wrapper: the shared `SidebarHeader`,
 * the same inner `flex items-stretch gap-1.5` row, the same
 * `@/components/ui/input` at the same size, the same magnifier laid over it
 * (file-explorer-tree.tsx). These two fields sit one swipe apart in the same
 * sidebar, and every part of them that was spelled out separately here had
 * already drifted — this panel's own wrapper gave it 6px under the tab switcher
 * where the explorer had 14px, so the switcher sat closer to one neighbour than
 * the other. `SidebarHeader` is where that rhythm lives now, so a third panel
 * inherits it rather than choosing again.
 *
 * ONE deliberate difference: no filter button beside the field. The explorer has
 * one because the file tree has real filters (ignored files, git status); this
 * panel has none, and a control that opens an empty menu is worse than no
 * control. The `flex-1` on the field's own wrapper is what lets it take the
 * width the button would have had.
 *
 * No scope control either. The panel is workspace-scoped by construction, and on
 * a project home that workspace already IS the long list this field exists for —
 * so a chip offering to "widen" would name a scope the panel cannot show.
 *
 * Nothing is drawn under the field. A count line was tried and removed: it cost
 * a row of height whenever the field had anything in it, and it advertised
 * `esc to clear` — a shortcut the field still honours, which no one needs told.
 */
export function AgentChatsSearch({ value, onChange }: AgentChatsSearchProps) {
  return (
    <SidebarHeader>
      <div className="flex items-stretch gap-1.5">
        <span className="relative flex min-w-0 flex-1 items-center">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute start-2.5 z-10 size-3.5 text-muted-foreground/72"
          />
          <Input
            nativeInput
            value={value}
            onChange={(e) => onChange(e.target.value)}
            size="sm"
            placeholder="Search chats"
            className="ps-5"
            name="chat-tree-filter"
            aria-label="Search chats"
            aria-controls="chat-tree-results"
            autoCapitalize="none"
            autoComplete="off"
            autoCorrect="off"
            spellCheck="false"
            onKeyDown={(e) => {
              // Stopped here so Escape clears the field rather than reaching the
              // pane/dialog handlers above it — inside a search box, Escape means
              // "clear", and only once it is already empty should it mean
              // anything else. (The explorer's Escape CLOSES its field instead:
              // that one is a toggled affordance, this one is permanent, so
              // there is nothing here to close.)
              if (e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation()
                onChange('')
              }
            }}
          />
        </span>
      </div>
    </SidebarHeader>
  )
}

import { SidebarTreeRow } from '@/components/sidebar/sidebar-tree'
import { useIsTouchDevice } from '@/hooks/use-is-touch-device'
import { useActiveSessionList, viewersLabel } from '@/core/entities/active-sessions'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

interface ActiveSessionsSectionProps {
  /** One project's rows: the section is a view over them, never a second tree. */
  rows: readonly SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
}

// Agents running now, pinned above the tree on touch so a phone finds a session it left running.
// Each entry is the chat's ordinary tree row (same component and menu), plus who is attached.
export function ActiveSessionsSection({
  rows,
  onOpen,
  onTrash,
  onCreate,
}: ActiveSessionsSectionProps) {
  const isTouch = useIsTouchDevice()
  const sessions = useActiveSessionList()
  const viewers = new Map(sessions.map((s) => [s.chatId, s.viewers]))
  const running = rows.filter((row) => row.kind !== 'folder' && viewers.has(row.id))
  if (!isTouch || running.length === 0) return null

  return (
    <section aria-label="Active" data-testid="active-sessions-section" className="pb-1">
      <h2 className="ui-font ui-text-sm px-3 py-1 text-muted-foreground">Active</h2>
      {running.map((row) => (
        <div key={row.id}>
          <SidebarTreeRow
            row={row}
            depth={0}
            onOpen={onOpen}
            onTrash={onTrash}
            onCreate={onCreate}
          />
          <p className="ui-font ui-text-sm truncate pr-3 pl-9 text-muted-foreground">
            {viewersLabel(viewers.get(row.id) ?? 0)}
          </p>
        </div>
      ))}
    </section>
  )
}

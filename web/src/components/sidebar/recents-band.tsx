import { X } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ROW_ACTIVE, ROW_SUB_ACTION_HOVER } from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { RecentsEntry, RecentsEntryState } from '@/features/panes/types/recents-entry'

// Re-exported for existing importers — the type itself lives in
// features/panes/types/recents-entry.ts so pane-slice.ts (a store) can hold
// `dormantArrangements: RecentsEntry[]` without importing from components/.
export type { RecentsEntry, RecentsEntryState }

interface RecentsBandProps {
  entries: RecentsEntry[]
  onFocus: (entry: RecentsEntry) => void
  /** No control renders for a 'working' entry — nothing calls this for one. */
  onClose: (entry: RecentsEntry) => void
}

// The close button sits OUTSIDE SidebarRow's own layout (absolute, over the
// row), so SidebarRow's `pr-2.5` (it has no trailing controls in this usage)
// leaves its `truncate` label free to render right up under it. Same problem
// `tab-bar-item.tsx` already solved for its own external close button —
// `Tab` reserves `pr-8` against a `!size-5` button at `right-1.5` (26px right
// extent, 6px of clearance). Ours is a 24px `ROW_SUB_ACTION_HOVER` button at
// `right-2.5` (34px right extent); `pr-10` (40px) keeps the same 6px margin.
const RECENTS_ROW_CLOSE_RESERVE = 'pr-10'

/**
 * §5: "what is up, and what is running." Every entry renders through
 * `SidebarRow` at `depth={0}` — no indent, no parentage, no chevron, no
 * second line (§5.1).
 *
 * Deliberately does NOT wire `onClose` into `SidebarRow`'s `onTrash` slot.
 * That slot hard-codes a Trash icon, destructive-red hover, and a
 * `Delete ${label}` aria-label — correct for the tree's destroy-the-chat verb,
 * but §5.4's "×" here means the opposite: end this view, never touch the
 * chat. Reusing `onTrash` would render a mislabelled delete affordance for a
 * non-destructive close. RecentsBand instead renders its own close control
 * beside the row, built from the same trailing-action tokens
 * (`ROW_SUB_ACTION_HOVER`) so it stays visually consistent without borrowing
 * the tree's destructive semantics.
 */
export function RecentsBand({ entries, onFocus, onClose }: RecentsBandProps) {
  // §5.7: no band until something is open — the empty state teaches the
  // tree/Recents pairing for free.
  if (entries.length === 0) return null

  return (
    <div data-testid="recents-band">
      <div className="flex h-[22px] items-center gap-1.5 px-1.5">
        <Separator className="flex-1" />
        <span className="shrink-0 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Recents
        </span>
      </div>
      {entries.map((entry) => (
        <RecentsEntryRow key={entry.id} entry={entry} onFocus={onFocus} onClose={onClose} />
      ))}
    </div>
  )
}

function RecentsEntryRow({
  entry,
  onFocus,
  onClose,
}: {
  entry: RecentsEntry
  onFocus: (entry: RecentsEntry) => void
  onClose: (entry: RecentsEntry) => void
}) {
  // §5.3/§5.6: chatIds.length decides the SHAPE — a shell around 2+ rows, or
  // a bare row for one. `state === 'live'` decides whether that shape is the
  // arrangement on screen right now. A literal 'set' with no 'live' is a
  // remembered (dormant) multi-chat view: the shell still draws — its own
  // ground, unlit — per §5.3 "at rest the shell and every member are empty".
  const isSet = entry.chatIds.length >= 2
  const isLive = entry.state === 'live'
  // §5.4: every row has a close control except the working one. There is
  // nothing left to close, and that absence is the "still running" signal.
  const canClose = entry.state !== 'working'

  return (
    <div
      className={cn(
        'group relative',
        isSet && 'mx-1.5 my-0.5 rounded-xl p-0.5',
        isSet && (isLive ? ROW_ACTIVE : 'bg-sidebar-element-idle'),
        !isSet && isLive && cn('mx-1.5 my-0.5 rounded-xl p-0.5', ROW_ACTIVE),
      )}
      data-testid={isSet ? `recents-set-${entry.id}` : undefined}
    >
      {entry.chatIds.map((chatId) => (
        <RecentsMemberRow
          key={chatId}
          chatId={chatId}
          hasView={isLive}
          reserveClose={canClose}
          onOpen={() => onFocus(entry)}
        />
      ))}
      {canClose && (
        <button
          type="button"
          data-control="close"
          data-testid={`recents-close-${entry.id}`}
          aria-label="Close view"
          className={cn(ROW_SUB_ACTION_HOVER, 'absolute right-2.5 top-1/2 -translate-y-1/2')}
          onClick={(e) => {
            e.stopPropagation()
            onClose(entry)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <X aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}
    </div>
  )
}

function RecentsMemberRow({
  chatId,
  hasView,
  reserveClose,
  onOpen,
}: {
  chatId: string
  hasView: boolean
  /** Whether this entry's row(s) sit under an overlaid close control — reserve
   *  room for it (see RECENTS_ROW_CLOSE_RESERVE) so a long title truncates
   *  before it, not before SidebarRow's own narrower built-in inset. */
  reserveClose: boolean
  onOpen: () => void
}) {
  const chat = useWorkspaceStoreContext((s) => s.agentChats.chats.find((c) => c.id === chatId))
  // Per-chat, narrow selector (copied verbatim from the tree's own pattern) —
  // the spinner rides the member wherever it lands (§5.6), independent of
  // which of the four band states its entry carries.
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)

  if (!chat) return null

  const row: SidebarRowType = {
    id: chat.id,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: chat.title,
    ownsWorktree: false,
    workspaceId: chat.workspaceId,
    working,
    hasView,
  }

  return (
    <div
      data-testid={`recents-row-${chat.id}`}
      className={cn(reserveClose && RECENTS_ROW_CLOSE_RESERVE)}
    >
      <SidebarRow row={row} depth={0} onOpen={onOpen} />
    </div>
  )
}

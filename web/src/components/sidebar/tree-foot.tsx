import type { ReactNode } from 'react'
import {
  DotsThreeIcon,
  FolderIcon,
  FolderOpenIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_GLYPH_BOX, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'

interface TreeFootProps {
  onNewThread: () => void
  onImportRepo: () => void
  onCreateFolder: () => void
  onDeleteSpace: () => void
}

function FootRow({
  icon,
  label,
  onClick,
}: {
  icon: ReactNode
  label: string
  onClick: (e: React.MouseEvent<HTMLButtonElement>) => void
}) {
  return (
    <button
      type="button"
      // Muted like the tab strip's "+" (tab-add-button.tsx): verbs, not content rows.
      className={cn(
        ROW_BASE,
        ROW_INACTIVE,
        'w-[calc(100%-0.75rem)] text-left text-muted-foreground',
      )}
      onClick={onClick}
    >
      <span className={ROW_GLYPH_BOX}>{icon}</span>
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}

/**
 * The space's own verbs, after its last tree row: a hairline, then New thread
 * and More… (a native menu). Inside the tree's scroller, so a short tree keeps
 * them right under it and an empty space shows them at the top.
 */
export function TreeFoot({
  onNewThread,
  onImportRepo,
  onCreateFolder,
  onDeleteSpace,
}: TreeFootProps) {
  const menu = useContextMenu()
  const items: ContextMenuItem[] = [
    { id: 'import-repo', label: 'Import a repo', icon: <FolderOpenIcon />, onClick: onImportRepo },
    {
      id: 'new-folder',
      label: 'Create a new folder',
      icon: <FolderIcon />,
      onClick: onCreateFolder,
    },
    {
      id: 'delete-space',
      label: 'Delete space',
      icon: <TrashIcon />,
      // The native OS menu has no destructive style; this marks the fallback popup.
      className:
        'text-destructive data-highlighted:bg-destructive/10 data-highlighted:text-destructive dark:data-highlighted:bg-destructive/20',
      onClick: onDeleteSpace,
    },
  ]

  return (
    <div data-testid="tree-foot">
      {/* Foreground at 15%, like view-row.tsx's separator: `--border` vanishes
          on the translucent sidebar. */}
      <div data-testid="tree-foot-divider" className="mx-3 my-1 h-px bg-foreground/15" />
      <FootRow
        icon={<PlusIcon aria-hidden="true" className="size-4" />}
        label="New thread"
        onClick={(e) => {
          e.currentTarget.blur()
          onNewThread()
        }}
      />
      <FootRow
        icon={<DotsThreeIcon aria-hidden="true" className="size-4" weight="bold" />}
        label="More…"
        onClick={(e) => {
          const rect = e.currentTarget.getBoundingClientRect()
          menu.openAt({ x: rect.left, y: rect.bottom + 4 })
        }}
      />
      {menu.isOpen && (
        <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
      )}
    </div>
  )
}

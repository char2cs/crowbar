import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_HAS_VIEW_IDLE,
  ROW_INACTIVE,
  ROW_INDENT_TRANSITION,
  ROW_NEST_TARGET,
  ROW_SUB_ACTION_HOVER,
  ROW_SUB_ACTION_INVERTED,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { formatChangeCount } from '@/components/layout/format-change-count'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { performRenameRow } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { BranchOutInput } from '@/components/sidebar/branch-out-input'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import { SidebarRowActions } from '@/components/sidebar/sidebar-row-actions'
import { SidebarRowGlyph } from '@/components/sidebar/sidebar-row-glyph'
import { OperationSidebarRow } from '@/components/sidebar/operation-sidebar-row'
import { RemovingSidebarRow } from '@/components/sidebar/sidebar-row-removing'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import {
  rowBoxStyle,
  rowGlyphStyle,
  rowIndent,
  rowSubStyle,
  rowTitleStyle,
  useRowMetrics,
  type RowMetrics,
} from '@/components/sidebar/lib/row-metrics'

interface SidebarRowProps {
  row: SidebarRowType
  depth: number
  onOpen: (id: string) => void
  onTrash?: (id: string) => void
  onClose?: (id: string) => void
  onCreate?: (id: string, kind: 'workspace' | 'thread') => void
  onToggleFold?: (id: string) => void
  folded?: boolean
  dragProps?: Record<string, string | undefined>
  isDragging?: boolean
  isNestTarget?: boolean
  onPointerDownDrag?: (e: React.PointerEvent) => void
  inlineRenameDisabled?: boolean
  hasViewIdle?: boolean
  activeGround?: boolean
  tab?: 'tab' | 'tab-member'
}

export function SidebarRow({
  row,
  depth,
  onOpen,
  onTrash,
  onClose,
  onCreate,
  onToggleFold,
  folded,
  dragProps,
  isDragging,
  isNestTarget,
  onPointerDownDrag,
  inlineRenameDisabled,
  hasViewIdle,
  activeGround,
  tab,
}: SidebarRowProps) {
  const metrics = useRowMetrics()
  const isThisRowRenaming = useTreeUi(
    (s) =>
      s.renaming === row.id &&
      (inlineRenameDisabled ? s.renamingRole === 'recents' : s.renamingRole !== 'recents'),
  )
  const branching = useTreeUi((s) => !inlineRenameDisabled && s.branching === row.id)
  const pendingMove = useChatField(row.id, (c) =>
    c?.pendingMove ? (c.pendingMove.label ?? '') : null,
  )
  if (row.operation || row.draft) {
    return <OperationSidebarRow row={row} depth={depth} />
  }
  if (row.removal) {
    return <RemovingSidebarRow row={row} depth={depth} removal={row.removal} />
  }
  const isProjectHome = row.kind === 'branch' && row.repoIcon !== undefined && !inlineRenameDisabled
  const expanded = !folded
  const renaming = isThisRowRenaming
  const editing = renaming || branching
  const showBranchSecondLine =
    !tab &&
    row.kind === 'branch' &&
    row.repoIcon === undefined &&
    !row.locked &&
    !!row.branchName &&
    row.label !== row.branchName
  const subActionClass = cn(ROW_SUB_ACTION_HOVER, activeGround && ROW_SUB_ACTION_INVERTED)
  const isReadOnlyHeader = !isProjectHome && row.kind === 'branch' && !!row.repoIcon

  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: rowIndent(depth, metrics) }}>
      <div
        role={tab ? 'tab' : 'treeitem'}
        aria-level={tab ? undefined : depth + 1}
        aria-selected={tab ? !row.hasView : undefined}
        aria-label={row.label}
        tabIndex={0}
        data-sidebar-row-id={row.id}
        {...dragProps}
        style={rowBoxStyle(metrics)}
        className={cn(
          ROW_BASE,
          isNestTarget ? ROW_NEST_TARGET : ROW_INACTIVE,
          !isNestTarget && activeGround && 'text-foreground-inverse',
          !isNestTarget && hasViewIdle && row.hasView && ROW_HAS_VIEW_IDLE,
          isDragging && 'opacity-40',
          'group',
          tab && 'mx-0 my-0',
          tab && 'h-8 px-1',
        )}
        onClick={(e) => {
          if (editing) return
          e.currentTarget.blur()
          onOpen(row.id)
        }}
        onPointerDown={editing ? undefined : onPointerDownDrag}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen(row.id)
          }
        }}
      >
        {(tab !== 'tab-member' || row.working) && (
          <span
            className={cn(
              ROW_GLYPH_BOX,
              isProjectHome && 'size-5',
              metrics && '[&_svg]:size-full',
              metrics && isReadOnlyHeader && '[&>*]:size-full',
            )}
            style={rowGlyphStyle(metrics)}
          >
            <SidebarRowGlyph
              row={row}
              isProjectHome={isProjectHome}
              readOnlyHeader={isReadOnlyHeader}
              expanded={expanded}
              activeGround={activeGround}
            />
          </span>
        )}

        {branching ? (
          <BranchOutInput row={row} />
        ) : renaming ? (
          <InlineRenameInput
            defaultValue={row.label}
            mono={row.kind === 'branch' && !showBranchSecondLine && !tab}
            onConfirm={(name) => {
              treeUiStore.getState().stopRenaming()
              if (name !== row.label) void performRenameRow(row.id, name)
            }}
            onCancel={() => treeUiStore.getState().stopRenaming()}
          />
        ) : showBranchSecondLine ? (
          <span
            data-sidebar-row-label=""
            className={cn(
              'flex min-w-0 flex-1 flex-col justify-center',
              row.hasView && !hasViewIdle && !activeGround && 'text-muted-foreground',
            )}
          >
            <span
              className={cn('truncate', row.labelProvisional && 'italic')}
              style={rowTitleStyle(metrics)}
            >
              {row.label}
            </span>
            <BranchSecondLine row={row} activeGround={activeGround} metrics={metrics} />
          </span>
        ) : pendingMove !== null && !tab ? (
          <span data-sidebar-row-label="" className="flex min-w-0 flex-1 flex-col justify-center">
            <span className="truncate" style={rowTitleStyle(metrics)}>
              {row.label}
            </span>
            <span
              className={cn(ROW_SUBLABEL, activeGround && 'text-foreground-inverse')}
              style={rowSubStyle(metrics)}
            >
              {pendingMove ? `Moving to ${pendingMove} after this turn` : 'Moving after this turn'}
            </span>
          </span>
        ) : (
          <span
            data-sidebar-row-label=""
            className={cn(
              'min-w-0 flex-1 truncate',
              row.kind === 'branch' && !tab && 'font-mono',
              row.labelProvisional && 'italic',
              row.hasView && !hasViewIdle && !activeGround && 'text-muted-foreground',
            )}
          >
            {row.label}
          </span>
        )}

        {!branching && (
          <SidebarRowActions
            row={row}
            isProjectHome={isProjectHome}
            expanded={expanded}
            movePending={pendingMove !== null}
            subActionClass={subActionClass}
            onCreate={onCreate}
            onTrash={onTrash}
            onClose={onClose}
            onToggleFold={onToggleFold}
          />
        )}
      </div>
    </div>
  )
}

function BranchSecondLine({
  row,
  activeGround,
  metrics,
}: {
  row: SidebarRowType
  metrics: RowMetrics | null
  activeGround?: boolean
}) {
  const added = row.added ?? 0
  const deleted = row.deleted ?? 0
  return (
    <span
      className={cn(ROW_SUBLABEL, activeGround && 'text-foreground-inverse')}
      style={rowSubStyle(metrics)}
    >
      {row.branchName}
      {(added > 0 || deleted > 0) && ' '}
      {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
      {added > 0 && deleted > 0 && ' '}
      {deleted > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>}
    </span>
  )
}

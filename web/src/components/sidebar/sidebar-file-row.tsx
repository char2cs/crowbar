import '@/features/file-explorer/file-explorer/styles/file-explorer-tree.css'
import type React from 'react'
import { formatChangeCount } from '@/components/layout/format-change-count'
import {
  ROW_GLYPH_BOX,
  ROW_HAS_VIEW_IDLE,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_NEST_TARGET,
  ROW_SHAPE,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { cn } from '@/lib/utils'

/** Pitch of every file-list row in the sidebar; virtualisers estimate with it. */
export const SIDEBAR_FILE_ROW_HEIGHT = 26
export const SIDEBAR_FILE_ROW_ICON_SIZE = 14
/** ROW_SHAPE's own `px-1.5`, so a depth-0 label starts where a chat row's does. */
export const SIDEBAR_FILE_ROW_BASE_INDENT = 6

export interface TreeGuideTarget {
  path: string
  name: string
  isDir: boolean
  isActive: boolean
}

interface TreeGuidesProps {
  depth: number
  baseIndent: number
  indentSize: number
  previousDepth: number
  nextDepth: number
  /** The ancestor each guide belongs to, so the tree's delegated handlers can act on it. */
  targets?: ReadonlyArray<TreeGuideTarget | null>
}

/**
 * The vertical rules connecting a nested row to its ancestors, one per level.
 * Inset at the top where a level begins and at the bottom where it ends.
 */
export function TreeGuides({
  depth,
  baseIndent,
  indentSize,
  previousDepth,
  nextDepth,
  targets,
}: TreeGuidesProps) {
  if (depth <= 0) return null
  return (
    <div className="file-tree-guides">
      {Array.from({ length: depth }, (_, level) => {
        const target = targets?.[level]
        return (
          <span
            key={level}
            className="file-tree-guide"
            data-file-path={target?.path}
            data-is-dir={target?.isDir}
            data-path={target?.path}
            data-active={target?.isActive ? 'true' : undefined}
            title={target?.name}
            style={{
              left: `calc(${baseIndent + level * indentSize}px + var(--file-tree-guide-icon-offset, 7px))`,
              top: previousDepth <= level ? '4px' : '0',
              bottom: nextDepth <= level ? '4px' : '0',
            }}
          />
        )
      })}
    </div>
  )
}

type SidebarFileRowProps = Omit<React.ComponentProps<'button'>, 'children'> & {
  depth?: number
  indentSize?: number
  baseIndent?: number
  height?: number
  selected?: boolean
  /** A drop would land inside this row (same nest signal as the chat tree). */
  dropTarget?: boolean
  /** Slot before the icon column, e.g. a disclosure caret. */
  leading?: React.ReactNode
  icon?: React.ReactNode
  label?: React.ReactNode
  labelClassName?: string
  /** Inline right after the label, e.g. a git status letter. */
  badge?: React.ReactNode
  /** Dimmed text after the label, e.g. a file's directory. */
  detail?: React.ReactNode
  /** Right-aligned slot, e.g. +N -N counts or a pill. */
  trailing?: React.ReactNode
  /** Replaces label/badge/detail/trailing with an input; the row is then not a button. */
  editor?: React.ReactNode
  /** Indent guides drawn behind the row. */
  guides?: React.ReactNode
  containerClassName?: string
}

/**
 * The dense sidebar row every file list draws through (Files explorer, Git
 * panel): the chat row's capsule, type and icon column at file-tree pitch.
 */
export function SidebarFileRow({
  depth = 0,
  indentSize = ROW_INDENT_STEP,
  baseIndent = SIDEBAR_FILE_ROW_BASE_INDENT,
  height = SIDEBAR_FILE_ROW_HEIGHT,
  selected = false,
  dropTarget = false,
  leading,
  icon,
  label,
  labelClassName,
  badge,
  detail,
  trailing,
  editor,
  guides,
  containerClassName,
  className,
  style,
  ...buttonProps
}: SidebarFileRowProps) {
  const rowClassName = cn(
    ROW_SHAPE,
    'file-tree-row w-full whitespace-nowrap text-left',
    dropTarget ? ROW_NEST_TARGET : ROW_INACTIVE,
    selected && !dropTarget && ROW_HAS_VIEW_IDLE,
    editor !== undefined && 'cursor-default',
    className,
  )
  const rowStyle = { height, paddingLeft: baseIndent + depth * indentSize, ...style }
  const iconColumn = icon !== undefined && <span className={ROW_GLYPH_BOX}>{icon}</span>

  return (
    <div className={cn('file-tree-item w-full', containerClassName)} data-depth={depth}>
      {guides}
      {editor !== undefined ? (
        <div className={rowClassName} style={rowStyle}>
          {leading}
          {iconColumn}
          {editor}
        </div>
      ) : (
        <button type="button" className={rowClassName} style={rowStyle} {...buttonProps}>
          {leading}
          {iconColumn}
          <span className={cn('min-w-0 truncate', labelClassName)}>{label}</span>
          {badge}
          {Boolean(detail) && (
            <span className="min-w-0 truncate text-muted-foreground">{detail}</span>
          )}
          {Boolean(trailing) && (
            <span className="ml-auto flex shrink-0 items-center gap-1.5 pl-1.5">{trailing}</span>
          )}
        </button>
      )}
    </div>
  )
}

/** A row's trailing `+N -N`, in the chat tree's count tokens and formatter. */
export function SidebarFileRowCounts({
  additions,
  deletions,
  compact = false,
}: {
  additions: number
  deletions: number
  compact?: boolean
}) {
  if (additions <= 0 && deletions <= 0) return null
  return (
    <span
      className={cn(
        'flex items-center font-mono text-[11px] tabular-nums',
        compact ? 'gap-0.5' : 'gap-1',
      )}
    >
      {additions > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(additions)}</span>}
      {deletions > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deletions)}</span>}
    </span>
  )
}

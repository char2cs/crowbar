import '@/features/file-explorer/styles/file-explorer-tree.css'
import type React from 'react'
import { forwardRef } from 'react'
import { cn } from '@/lib/utils'
import { TreeRow } from './tree-row'

export const SIDEBAR_TREE_BASE_INDENT = 10
export const SIDEBAR_TREE_INDENT_SIZE = 14
export const SIDEBAR_TREE_ICON_SIZE = 14

interface TreeGuidesProps {
  depth: number
  baseIndent: number
  indentSize: number
  previousDepth: number
  nextDepth: number
}

/**
 * The vertical rules connecting a nested row to its ancestors — one per level.
 *
 * A guide is inset at the top when the row begins a level (no sibling above it at that
 * level) and inset at the bottom when it ends one, which rounds off each run of guides
 * instead of butting them against neighbouring rows.
 */
function TreeGuides({ depth, baseIndent, indentSize, previousDepth, nextDepth }: TreeGuidesProps) {
  if (depth <= 0) {
    return null
  }

  return (
    <div className="file-tree-guides">
      {Array.from({ length: depth }, (_, level) => (
        <span
          key={level}
          className="file-tree-guide"
          data-oracle-id={`git-row-guide-${level}`}
          style={{
            left: `calc(${baseIndent + level * indentSize}px + var(--file-tree-guide-icon-offset, 7px))`,
            top: previousDepth <= level ? '4px' : '0',
            bottom: nextDepth <= level ? '4px' : '0',
          }}
        />
      ))}
    </div>
  )
}

type SidebarTreeRowProps = React.ComponentPropsWithoutRef<'button'> & {
  active?: boolean
  depth?: number
  indentSize?: number
  baseIndent?: number
  /** Depth of the adjacent rows, used to cap the indent guides at each run's edges. */
  previousDepth?: number
  nextDepth?: number
  containerClassName?: string
}

/**
 * A {@link TreeRow} wrapped with the indent guides used by the sidebar trees.
 *
 * Carries the `data-oracle-id` anchors for the row container, the row button and
 * each indent guide — see `native/oracle/ANCHORS.md`. They are inert markers with
 * no styling or behaviour attached and ship in production by design.
 */
export const SidebarTreeRow = forwardRef<HTMLButtonElement, SidebarTreeRowProps>(
  function SidebarTreeRow(
    {
      active = false,
      depth = 0,
      indentSize = SIDEBAR_TREE_INDENT_SIZE,
      baseIndent = SIDEBAR_TREE_BASE_INDENT,
      previousDepth = depth,
      nextDepth = depth,
      containerClassName,
      className,
      children,
      ...props
    },
    ref,
  ) {
    return (
      <div
        className={cn('file-tree-item w-full', containerClassName)}
        data-active={active ? 'true' : undefined}
        data-depth={depth}
        data-oracle-id="git-row-item"
      >
        <TreeGuides
          depth={depth}
          baseIndent={baseIndent}
          indentSize={indentSize}
          previousDepth={previousDepth}
          nextDepth={nextDepth}
        />
        <TreeRow
          ref={ref}
          depth={depth}
          indentSize={indentSize}
          baseIndent={baseIndent}
          className={cn('h-6 gap-1.5 border border-transparent px-1.5 py-1', className)}
          data-oracle-id="git-row-button"
          {...props}
        >
          {children}
        </TreeRow>
      </div>
    )
  },
)

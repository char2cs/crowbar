// copied from Athas — no shadcn/ui equivalent
import '@/features/file-explorer/styles/file-explorer-tree.css'
import type React from 'react'
import { forwardRef } from 'react'
import { cn } from '@/lib/utils'
import { TreeRow } from './tree-row'

export const SIDEBAR_TREE_BASE_INDENT = 10
export const SIDEBAR_TREE_INDENT_SIZE = 14
export const SIDEBAR_TREE_ICON_SIZE = 14

interface SidebarTreeGuidesProps {
  depth: number
  baseIndent?: number
  indentSize?: number
  previousDepth?: number
  nextDepth?: number
}

function SidebarTreeGuides({
  depth,
  baseIndent = SIDEBAR_TREE_BASE_INDENT,
  indentSize = SIDEBAR_TREE_INDENT_SIZE,
  previousDepth = depth,
  nextDepth = depth,
}: SidebarTreeGuidesProps) {
  if (depth <= 0) return null

  return (
    <div className="file-tree-guides">
      {Array.from({ length: depth }, (_, level) => {
        const startsHere = previousDepth <= level
        const endsHere = nextDepth <= level

        return (
          <span
            key={level}
            className="file-tree-guide"
            style={{
              left: `calc(${baseIndent + level * indentSize}px + var(--file-tree-guide-icon-offset, 7px))`,
              top: startsHere ? '4px' : '0',
              bottom: endsHere ? '4px' : '0',
            }}
          />
        )
      })}
    </div>
  )
}

type SidebarTreeRowProps = React.ComponentPropsWithoutRef<'button'> & {
  active?: boolean
  depth?: number
  indentSize?: number
  baseIndent?: number
  previousDepth?: number
  nextDepth?: number
  containerClassName?: string
}

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
      >
        <SidebarTreeGuides
          depth={depth}
          baseIndent={baseIndent}
          indentSize={indentSize}
          previousDepth={previousDepth}
          nextDepth={nextDepth}
        />
        <TreeRow
          ref={ref}
          active={false}
          depth={depth}
          indentSize={indentSize}
          baseIndent={baseIndent}
          className={cn('h-6 gap-1.5 border border-transparent px-1.5 py-1', className)}
          {...props}
        >
          {children}
        </TreeRow>
      </div>
    )
  },
)

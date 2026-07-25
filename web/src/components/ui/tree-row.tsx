import type React from 'react'
import { cn } from '@/lib/utils'

export const TREE_ROW_BASE_INDENT = 14
export const TREE_ROW_INDENT_SIZE = 14

type TreeRowProps = React.ComponentProps<'button'> & {
  active?: boolean
  /** Nesting level; each level adds `indentSize` to the row's leading padding. */
  depth?: number
  indentSize?: number
  baseIndent?: number
}

/**
 * A single selectable row in a tree. Indentation is applied as padding rather than a
 * spacer element so the row's hover and selection background span the full width.
 */
export function TreeRow({
  active = false,
  depth = 0,
  indentSize = TREE_ROW_INDENT_SIZE,
  baseIndent = TREE_ROW_BASE_INDENT,
  className,
  style,
  children,
  ...props
}: TreeRowProps) {
  return (
    <button
      type="button"
      className={cn(
        'file-tree-row flex w-full min-w-max cursor-pointer select-none items-center',
        'whitespace-nowrap rounded-md border-none bg-transparent text-left text-sm text-foreground',
        'outline-none transition-colors duration-150 hover:bg-muted focus:outline-none',
        active && 'bg-accent/20',
        className,
      )}
      style={{ paddingLeft: baseIndent + depth * indentSize, ...style }}
      {...props}
    >
      {children}
    </button>
  )
}

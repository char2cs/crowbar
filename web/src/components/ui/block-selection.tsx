'use client'

import { useBlockSelected } from '@platejs/selection/react'
import { cva } from 'class-variance-authority'
import type { PlateElementProps } from 'platejs/react'

export const blockSelectionVariants = cva(
  'pointer-events-none absolute inset-0 z-1 bg-brand/[.13] transition-opacity',
  {
    defaultVariants: {
      active: true,
    },
    variants: {
      active: {
        false: 'opacity-0',
        true: 'opacity-100',
      },
    },
  },
)

// The overlay `BlockSelectionKit` (`components/editor/plugins/block-selection-kit.tsx`)
// renders below every selectable block, painted on top of it while the block
// is part of the drag/menu multi-selection. Added here alongside the
// pre-existing `blockSelectionVariants` (already consumed by table-node.tsx
// for cell selection) — this component itself was never wired up before now.
export function BlockSelection(props: PlateElementProps) {
  const isBlockSelected = useBlockSelected()

  if (!isBlockSelected || props.plugin.key === 'tr' || props.plugin.key === 'table') return null

  return (
    <div
      className={blockSelectionVariants({
        active: isBlockSelected,
      })}
      data-slot="block-selection"
    />
  )
}

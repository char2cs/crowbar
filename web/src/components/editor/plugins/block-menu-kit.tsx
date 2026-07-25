// From the Plate registry (`https://platejs.org/r/block-menu-kit.json`),
// verbatim — the actual composition is trivial; the trimming happened in the
// two files it pulls in (`block-selection-kit.tsx`, `block-context-menu.tsx`).
'use client'

import { BlockMenuPlugin } from '@platejs/selection/react'

import { BlockContextMenu } from '@/components/ui/block-context-menu'

import { BlockSelectionKit } from './block-selection-kit'

export const BlockMenuKit = [
  ...BlockSelectionKit,
  BlockMenuPlugin.configure({
    render: { aboveEditable: BlockContextMenu },
  }),
]

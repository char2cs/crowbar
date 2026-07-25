// Adapted from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `block-selection-kit`). Trimmed: upstream also wires a `mod+j` hotkey
// through `@platejs/ai`'s `AIChatPlugin` to open an AI chat panel — this app
// has no AI kit installed, so that option (and the `@platejs/ai` import) is
// dropped; `BlockSelectionKit` here only turns on the selection overlay
// `block-menu-kit.tsx` and the drag handle (`dnd-kit.tsx`) both depend on.
'use client'

import { BlockSelectionPlugin } from '@platejs/selection/react'
import { getPluginTypes, KEYS } from 'platejs'

import { BlockSelection } from '@/components/ui/block-selection'

export const hasSelectableClass = ({
  attributes,
  className,
}: {
  attributes: { className?: string }
  className?: string
}) => [className, attributes.className].filter(Boolean).join(' ').includes('slate-selectable')

export const BlockSelectionKit = [
  BlockSelectionPlugin.configure(({ editor }) => ({
    options: {
      enableContextMenu: true,
      isSelectable: (element) =>
        !getPluginTypes(editor, [KEYS.column, KEYS.codeLine, KEYS.td]).includes(element.type),
    },
    render: {
      belowRootNodes: (props) => {
        if (!hasSelectableClass(props)) return null

        // `belowRootNodes`'s `props` is generic over THIS plugin's own config
        // (`PluginConfig<'blockSelection', ...>`), while `BlockSelection`
        // takes the plain `PlateElementProps` — same shape at runtime, but
        // TS won't unify the two generic instantiations structurally. Same
        // escape hatch the upstream registry file uses.
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        return <BlockSelection {...(props as any)} />
      },
    },
  })),
]

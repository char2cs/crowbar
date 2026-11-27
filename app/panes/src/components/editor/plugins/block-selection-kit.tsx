'use client'

import { BlockSelectionPlugin } from '@platejs/selection/react'
import { getPluginTypes, KEYS } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

import { BlockSelection } from '@/components/ui/block-selection'

const hasSelectableClass = ({
  attributes,
  className,
}: {
  attributes: { className?: string }
  className?: string
}) => [className, attributes.className].filter(Boolean).join(' ').includes('slate-selectable')

const SelectAllDocument = createPlatePlugin({ key: 'selectAllDocument' }).overrideEditor(
  ({ editor, tf: { selectAll } }) => ({
    transforms: {
      selectAll: () => {
        if (selectAll()) return true
        editor.tf.select([])
        return true
      },
    },
  }),
)

export const BlockSelectionKit = [
  BlockSelectionPlugin.configure(({ editor }) => ({
    options: {
      enableContextMenu: true,
      disableSelectAll: true,
      isSelectable: (element) =>
        !getPluginTypes(editor, [KEYS.column, KEYS.codeLine, KEYS.td]).includes(element.type),
    },
    render: {
      belowRootNodes: (props) => {
        if (!hasSelectableClass(props)) return null

        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the two plugin generics do not unify structurally
        return <BlockSelection {...(props as any)} />
      },
    },
  })),
  SelectAllDocument,
]

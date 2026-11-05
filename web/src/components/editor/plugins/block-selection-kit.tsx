// Adapted from the Plate registry (block-menu-kit.json -> `block-selection-kit`), minus the
// `@platejs/ai` mod+j chat this app lacks; the overlay that block-menu-kit and the drag handle
// depend on. Added: Cmd/Ctrl+A is a document-wide TEXT select-all (SelectAllDocument).
'use client'

import { BlockSelectionPlugin } from '@platejs/selection/react'
import { getPluginTypes, KEYS } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

import { BlockSelection } from '@/components/ui/block-selection'

export const hasSelectableClass = ({
  attributes,
  className,
}: {
  attributes: { className?: string }
  className?: string
}) => [className, attributes.className].filter(Boolean).join(' ').includes('slate-selectable')

// Cmd/Ctrl+A selects the whole DOCUMENT as text. Runs LAST (after BlockSelectionPlugin, hence
// this array) so code-block and table scoping claim it first. Returns `true` so the caller
// preventDefaults: the browser default does nothing under a synthesized keydown.
const SelectAllDocument = createPlatePlugin({ key: 'selectAllDocument' }).overrideEditor(
  ({ editor, tf: { selectAll } }) => ({
    transforms: {
      selectAll: () => {
        if (selectAll()) return true
        // `[]` is the editor node itself; Slate resolves a path to that node's
        // full range, so this is start-of-document to end-of-document.
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
      // Cmd/Ctrl+A goes to the text selection (SelectAllDocument): a BLOCK selection's keymap ignores
      // left/right, so a fully selected document would never collapse.
      disableSelectAll: true,
      isSelectable: (element) =>
        !getPluginTypes(editor, [KEYS.column, KEYS.codeLine, KEYS.td]).includes(element.type),
    },
    render: {
      belowRootNodes: (props) => {
        if (!hasSelectableClass(props)) return null

        // `belowRootNodes`' props are generic over this plugin's config while `BlockSelection` takes plain
        // `PlateElementProps`: same runtime shape, but TS won't unify them (upstream casts too).
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the two plugin generics do not unify structurally
        return <BlockSelection {...(props as any)} />
      },
    },
  })),
  SelectAllDocument,
]

// Adapted from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `block-selection-kit`). Trimmed: upstream also wires a `mod+j` hotkey
// through `@platejs/ai`'s `AIChatPlugin` to open an AI chat panel — this app
// has no AI kit installed, so that option (and the `@platejs/ai` import) is
// dropped; `BlockSelectionKit` here turns on the selection overlay
// `block-menu-kit.tsx` and the drag handle (`dnd-kit.tsx`) both depend on.
// Added on top of the registry file: Cmd/Ctrl+A is kept out of block selection
// and made a document-wide TEXT select-all instead (see SelectAllDocument).
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

/**
 * Cmd/Ctrl+A selects the whole DOCUMENT as text — Obsidian's behaviour, and
 * every other markdown editor's.
 *
 * Plate leaves `editor.tf.selectAll` as a `() => false` stub (@platejs/slate)
 * and lets plugins claim it. Two of them legitimately do: with the caret in a
 * code block the first Cmd+A scopes to that fence (Obsidian does the same),
 * and in a table it scopes to the whole table. So this runs LAST in the chain
 * and only acts once those have passed. It must also be ordered after
 * `BlockSelectionPlugin` above, which is why it lives in this array rather
 * than in its own kit.
 *
 * Returning `true` is what makes the keystroke deterministic: the caller
 * (`SlateReactExtensionPlugin`) only calls `preventDefault()` on a truthy
 * return. Falling through to the browser's own select-all would look identical
 * in a browser and do NOTHING under a synthesized keydown, and relying on a
 * default action for a core editing command is not something to leave to the
 * host anyway.
 *
 * A plain text range is the entire point: with no block selection active,
 * arrows, shift+arrows, typing-over and copy are all the caret's own
 * behaviour, so left/right collapse the selection the way they do everywhere.
 */
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
      // Hand Cmd/Ctrl+A back to the text selection — SelectAllDocument above
      // has the whole story. Without this the keystroke goes into a BLOCK
      // selection, whose keymap lives on a hidden input in `document.body` and
      // handles Escape/Enter/Backspace/up/down and nothing else: left and
      // right arrow land on a handler that ignores them, so a fully selected
      // document simply will not collapse.
      disableSelectAll: true,
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
  SelectAllDocument,
]

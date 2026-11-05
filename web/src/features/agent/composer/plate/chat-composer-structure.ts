import { RangeApi, type TCodeBlockElement, type TElement } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin } from 'platejs/react'
import { parseAttachmentLang } from '@/features/agent/composer/plate/attachments/attachment-lang'

/** The one node type the box can always be typed into. */
const LINE = 'p'
const FENCE = 'code_block'
const IMAGE = 'img'

const emptyLine = (): TElement => ({ type: LINE, children: [{ text: '' }] })

/** Top level only, so a table cell or nested list keeps ordinary merge behaviour. */
function topLevelBlockStart(editor: PlateEditor): number | null {
  const { selection } = editor
  if (!selection || !RangeApi.isCollapsed(selection)) return null
  const entry = editor.api.above({ match: (n) => editor.api.isBlock(n), mode: 'lowest' })
  if (!entry || entry[1].length !== 1) return null
  return editor.api.isStart(selection.anchor, entry[1]) ? entry[1][0] : null
}

const fenceLang = (node: TElement | undefined): string | undefined =>
  node?.type === FENCE ? ((node as TCodeBlockElement).lang ?? undefined) : undefined

/** No editable surface: a void image, or a fence whose code body is hidden — a
 *  caret there would silently edit the payload. */
function isAttachmentBlock(editor: PlateEditor, node: TElement | undefined): boolean {
  if (!node) return false
  return editor.api.isVoid(node) || !!parseAttachmentLang(fenceLang(node))
}

/** An excalidraw attachment is two nodes: the fence and the PNG after it. */
function attachmentStart(editor: PlateEditor, index: number): number {
  const isPngSibling =
    editor.children[index]?.type === IMAGE &&
    parseAttachmentLang(fenceLang(editor.children[index - 1]))?.kind === 'excalidraw'
  return isPngSibling ? index - 1 : index
}

/** Invariant: the last block is always a typeable line — Slate never normalizes an
 *  empty root. Backspace against an attachment removes it. Composer editor only. */
export const chatComposerStructurePlugin = createPlatePlugin({
  key: 'agent-chat-structure',
  // Plate does not normalize a `value:` it is handed.
  transformInitialValue: ({ value }) =>
    value.at(-1)?.type === LINE ? value : [...value, emptyLine()],
}).overrideEditor(({ editor, tf: { deleteBackward, normalizeNode } }) => ({
  transforms: {
    deleteBackward(unit) {
      const index = topLevelBlockStart(editor)
      if (index !== null && index > 0 && isAttachmentBlock(editor, editor.children[index - 1])) {
        const first = attachmentStart(editor, index - 1)
        editor.tf.withoutNormalizing(() => {
          for (let at = index - 1; at >= first; at--) editor.tf.removeNodes({ at: [at] })
        })
        return
      }
      deleteBackward(unit)
    },
    normalizeNode(entry) {
      if (entry[1].length === 0) {
        const last = editor.children.at(-1)
        if (!last || last.type !== LINE) {
          editor.tf.insertNodes(emptyLine(), { at: [editor.children.length] })
          // An emptied editor is left with `selection: null`, which swallows the
          // next keystroke.
          if (!last && !editor.selection) {
            const start = editor.api.start([])
            if (start) editor.tf.select(start)
          }
          return
        }
      }
      normalizeNode(entry)
    },
  },
}))

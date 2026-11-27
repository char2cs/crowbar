import {
  KEYS,
  NodeApi,
  RangeApi,
  type Descendant,
  type TCodeBlockElement,
  type TElement,
} from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin } from 'platejs/react'
import { parseAttachmentLang } from '@/features/agent/composer/plate/attachments/attachment-lang'

const LINE = 'p'
const FENCE = 'code_block'
const IMAGE = 'img'

export function blankParagraph(node: Descendant): boolean {
  const markup = Object.keys(node).filter((key) => key !== 'id' && key !== 'children')
  if (!('type' in node) || node.type !== LINE || markup.length > 1) return false
  return NodeApi.string(node).trim() === ''
}

const emptyLine = (): TElement => ({ type: LINE, children: [{ text: '' }] })

function topLevelBlockStart(editor: PlateEditor): number | null {
  const { selection } = editor
  if (!selection || !RangeApi.isCollapsed(selection)) return null
  const entry = editor.api.above({ match: (n) => editor.api.isBlock(n), mode: 'lowest' })
  if (!entry || entry[1].length !== 1) return null
  return editor.api.isStart(selection.anchor, entry[1]) ? entry[1][0] : null
}

const fenceLang = (node: TElement | undefined): string | undefined =>
  node?.type === FENCE ? ((node as TCodeBlockElement).lang ?? undefined) : undefined

function isAttachmentBlock(editor: PlateEditor, node: TElement | undefined): boolean {
  if (!node) return false
  return editor.api.isVoid(node) || !!parseAttachmentLang(fenceLang(node))
}

function attachmentStart(editor: PlateEditor, index: number): number {
  const isPngSibling =
    editor.children[index]?.type === IMAGE &&
    parseAttachmentLang(fenceLang(editor.children[index - 1]))?.kind === 'excalidraw'
  return isPngSibling ? index - 1 : index
}

export const chatComposerStructurePlugin = createPlatePlugin({
  key: 'agent-chat-structure',
  transformInitialValue: ({ value }) =>
    value.at(-1)?.type === LINE ? value : [...value, emptyLine()],
}).overrideEditor(({ editor, tf: { deleteBackward, normalizeNode, tab } }) => ({
  transforms: {
    tab(options) {
      const block = editor.api.block()?.[0] as (TElement & { listStyleType?: string }) | undefined
      const inCode = editor.api.some({ match: { type: editor.getType(KEYS.codeBlock) } })
      if (!block?.listStyleType && !inCode) return false
      return tab(options)
    },
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
        const blank = editor.children.length > 0 && editor.children.every(blankParagraph)
        if (blank && (editor.children.length > 1 || NodeApi.string(editor.children[0]!) !== '')) {
          editor.tf.withoutNormalizing(() => {
            for (let at = editor.children.length - 1; at >= 0; at--) {
              editor.tf.removeNodes({ at: [at] })
            }
            editor.tf.insertNodes(emptyLine(), { at: [0] })
            editor.tf.select(editor.api.start([])!)
          })
          return
        }
        const last = editor.children.at(-1)
        if (!last || last.type !== LINE) {
          editor.tf.insertNodes(emptyLine(), { at: [editor.children.length] })
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

const TITLE = 'h1'

export function createDocumentTitlePlugin(isDocument: () => boolean) {
  return createPlatePlugin({ key: 'agent-chat-title' }).overrideEditor(
    ({ editor, tf: { insertBreak, normalizeNode } }) => ({
      transforms: {
        insertBreak() {
          const inTitle =
            isDocument() &&
            editor.selection?.anchor.path[0] === 0 &&
            editor.children[0]?.type === TITLE
          const next = editor.children[1]
          if (inTitle && next?.type === LINE && editor.api.isEnd(editor.selection!.anchor, [0])) {
            editor.tf.select(editor.api.start([1])!)
            return
          }
          insertBreak()
          if (inTitle && editor.children[1]?.type === TITLE) {
            editor.tf.setNodes({ type: LINE }, { at: [1] })
          }
        },
        normalizeNode(entry) {
          const first = editor.children[0]
          if (entry[1].length === 0 && isDocument() && first) {
            const text = NodeApi.string(first)
            const retype =
              first.type === LINE && text.trim() !== '' && !text.startsWith('/')
                ? TITLE
                : first.type === TITLE && text === ''
                  ? LINE
                  : null
            if (retype) {
              editor.tf.setNodes({ type: retype }, { at: [0] })
              return
            }
          }
          normalizeNode(entry)
        },
      },
    }),
  )
}

import {
  convertNodesDeserialize,
  getMergedOptionsDeserialize,
  MarkdownPlugin,
  markdownToAstProcessor,
} from '@platejs/markdown'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import { TextApi, type Descendant, type Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import { nodeKey, serializeOverSource, type MarkdownSource } from './markdown-source'

type PlateEditorPlugins = NonNullable<Parameters<typeof createPlateEditor>[0]>['plugins']
type HeadlessEditor = ReturnType<typeof createPlateEditor>

/** A parsed document: the editor value, and the source it maps back onto. */
export interface ParsedMarkdown {
  value: Value
  source: MarkdownSource
}

/** Markdown text <-> Plate value, for one specific plugin set. */
export interface MarkdownCodec {
  toValue(md: string): Value
  /** Like `toValue`, keeping where each block came from so `toMarkdown` can copy it back. */
  parse(md: string): ParsedMarkdown
  /** With `source`, blocks the value still holds unchanged are written back byte for byte. */
  toMarkdown(value: Value, source?: MarkdownSource): string
}

// `withoutMdx` keeps raw HTML as `html` nodes; the html->JSX transform would save `class` as
// `className` and corrupt a README's `<div class>` header. Serialize needs nothing:
// markdown-html-rules.ts emits the raw value verbatim.
const DESERIALIZE = { withoutMdx: true }

// A codec per plugin set, since the set defines which nodes exist (a missing one is dropped).
// Imports no plugin set, so the reduced comment set stays out of the full editor's chunks.
// Headless editors are built lazily once per direction: toMarkdown parks its doc in children.
export function createMarkdownCodec(plugins: PlateEditorPlugins): MarkdownCodec {
  let deserializeEditor: HeadlessEditor | undefined
  let serializeEditor: HeadlessEditor | undefined

  const serialize = (value: Descendant[]): string => {
    serializeEditor ??= createPlateEditor({ plugins })
    serializeEditor.children = value as Value
    return serializeEditor.getApi(MarkdownPlugin).markdown.serialize()
  }

  return {
    toValue(md: string): Value {
      deserializeEditor ??= createPlateEditor({ plugins })
      return deserializeEditor.getApi(MarkdownPlugin).markdown.deserialize(md, DESERIALIZE)
    },

    parse(md: string): ParsedMarkdown {
      deserializeEditor ??= createPlateEditor({ plugins })
      const options = getMergedOptionsDeserialize(deserializeEditor, DESERIALIZE)
      const root = markdownToAstProcessor(deserializeEditor, md, DESERIALIZE)
      const value: Value = []
      const source: MarkdownSource = { text: md, blocks: [] }
      for (const child of root.children) {
        // Same per-child conversion as a whole-document deserialize, one block at a time.
        const nodes = convertNodesDeserialize([child], {}, options).map((node) =>
          TextApi.isText(node) ? { type: 'p', children: [node] } : node,
        )
        const { start, end } = child.position ?? {}
        if (start?.offset !== undefined && end?.offset !== undefined) {
          source.blocks.push({ start: start.offset, end: end.offset, keys: nodes.map(nodeKey) })
        }
        value.push(...(nodes as Value))
      }
      return { value, source }
    },

    toMarkdown(value: Value, source?: MarkdownSource): string {
      if (!source) return serialize(value)
      return serializeOverSource(value, source, serialize)
    },
  }
}

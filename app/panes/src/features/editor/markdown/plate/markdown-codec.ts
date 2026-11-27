import {
  convertNodesDeserialize,
  getMergedOptionsDeserialize,
  MarkdownPlugin,
  markdownToAstProcessor,
} from '@platejs/markdown'
import { TextApi, type Descendant, type Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import { nodeKey, serializeOverSource, type MarkdownSource } from './markdown-source'

type PlateEditorPlugins = NonNullable<Parameters<typeof createPlateEditor>[0]>['plugins']
type HeadlessEditor = ReturnType<typeof createPlateEditor>

export interface ParsedMarkdown {
  value: Value
  source: MarkdownSource
}

export interface MarkdownCodec {
  toValue(md: string): Value
  parse(md: string): ParsedMarkdown
  toMarkdown(value: Value, source?: MarkdownSource): string
}

const DESERIALIZE = { withoutMdx: true }

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

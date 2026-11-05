import { MarkdownPlugin } from '@platejs/markdown'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import type { Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'

type PlateEditorPlugins = NonNullable<Parameters<typeof createPlateEditor>[0]>['plugins']
type HeadlessEditor = ReturnType<typeof createPlateEditor>

/** Markdown text <-> Plate value, for one specific plugin set. */
export interface MarkdownCodec {
  toValue(md: string): Value
  toMarkdown(value: Value): string
}

// A codec per plugin set, since the set defines which nodes exist (a missing one is dropped).
// Imports no plugin set, so the reduced comment set stays out of the full editor's chunks.
// Headless editors are built lazily once per direction: toMarkdown parks its doc in children.
export function createMarkdownCodec(plugins: PlateEditorPlugins): MarkdownCodec {
  let deserializeEditor: HeadlessEditor | undefined
  let serializeEditor: HeadlessEditor | undefined

  return {
    toValue(md: string): Value {
      deserializeEditor ??= createPlateEditor({ plugins })
      // `withoutMdx` keeps raw HTML as `html` nodes; the html->JSX transform would save
      // `class` as `className` and corrupt a README's `<div class>` header. Serialize needs
      // nothing: markdown-html-rules.ts emits the raw value verbatim.
      return deserializeEditor.getApi(MarkdownPlugin).markdown.deserialize(md, { withoutMdx: true })
    },

    toMarkdown(value: Value): string {
      serializeEditor ??= createPlateEditor({ plugins })
      serializeEditor.children = value
      return serializeEditor.getApi(MarkdownPlugin).markdown.serialize()
    },
  }
}

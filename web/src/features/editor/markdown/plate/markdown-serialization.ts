import { MarkdownPlugin } from '@platejs/markdown'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import type { Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import { markdownPlugins } from './markdown-plugins'

// Building a Plate editor instantiates and wires the WHOLE plugin set — it is
// not something to do per call. These two run on every pristine check and every
// debounced flush (several times a second while typing), so the headless
// editors are built once, lazily, and reused (M5).
//
// One instance per direction rather than a single shared editor:
// `plateValueToMarkdown` parks the document it is serializing in
// `editor.children`, and a deserialize sharing that instance would observe
// another call's leftovers. Neither function is reentrant (both are
// synchronous), so one instance each is enough. Lazy so importing this module
// costs nothing until a markdown buffer is actually opened.
type HeadlessEditor = ReturnType<typeof createPlateEditor>

let deserializeEditor: HeadlessEditor | undefined
let serializeEditor: HeadlessEditor | undefined

/** Markdown text -> Plate value. */
export function markdownToPlateValue(md: string): Value {
  deserializeEditor ??= createPlateEditor({ plugins: markdownPlugins })
  // `withoutMdx` keeps raw HTML as plain `html` mdast nodes instead of running
  // it through @platejs/markdown's html→JSX transform, which rewrites `class` →
  // `className` etc.; those mangled attributes would then be SAVED, corrupting a
  // README's `<div class="...">` header into invalid HTML. It's a deserialize-
  // only, per-call option (not part of the plugin config). The serialize side
  // needs nothing extra: markdown-html-rules.ts emits the raw value verbatim.
  return deserializeEditor.getApi(MarkdownPlugin).markdown.deserialize(md, { withoutMdx: true })
}

/** Plate value -> markdown text. */
export function plateValueToMarkdown(value: Value): string {
  serializeEditor ??= createPlateEditor({ plugins: markdownPlugins })
  serializeEditor.children = value
  return serializeEditor.getApi(MarkdownPlugin).markdown.serialize()
}

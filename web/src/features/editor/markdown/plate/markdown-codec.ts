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

/**
 * Builds a markdown codec bound to `plugins`.
 *
 * A codec is per-plugin-set rather than global because a plugin set defines
 * which node types EXIST: deserializing with a set that lacks a node drops it.
 * The full editor and the comment editor therefore each own one, and neither
 * can accidentally serialize through the other's rules.
 *
 * This module deliberately imports NO plugin set of its own. Both callers pull
 * their own in, and a shared import here would put every plugin either of them
 * registers into every chunk either of them reaches — which is precisely what
 * the reduced comment set exists to avoid (`comment-plugins.tsx` explains what
 * it is avoiding and why).
 *
 * Building a Plate editor instantiates and wires the WHOLE plugin set — it is
 * not something to do per call. These run on every pristine check and every
 * debounced flush (several times a second while typing), so the headless
 * editors are built once, lazily, and reused.
 *
 * One instance per DIRECTION rather than a single shared editor: `toMarkdown`
 * parks the document it is serializing in `editor.children`, and a deserialize
 * sharing that instance would observe another call's leftovers. Neither
 * function is reentrant (both are synchronous), so one instance each is enough.
 * Lazy so importing a codec costs nothing until something is actually edited.
 */
export function createMarkdownCodec(plugins: PlateEditorPlugins): MarkdownCodec {
  let deserializeEditor: HeadlessEditor | undefined
  let serializeEditor: HeadlessEditor | undefined

  return {
    toValue(md: string): Value {
      deserializeEditor ??= createPlateEditor({ plugins })
      // `withoutMdx` keeps raw HTML as plain `html` mdast nodes instead of
      // running it through @platejs/markdown's html->JSX transform, which
      // rewrites `class` -> `className` etc.; those mangled attributes would
      // then be SAVED, corrupting a README's `<div class="...">` header into
      // invalid HTML. It's a deserialize-only, per-call option (not part of
      // the plugin config). The serialize side needs nothing extra:
      // markdown-html-rules.ts emits the raw value verbatim.
      return deserializeEditor.getApi(MarkdownPlugin).markdown.deserialize(md, { withoutMdx: true })
    },

    toMarkdown(value: Value): string {
      serializeEditor ??= createPlateEditor({ plugins })
      serializeEditor.children = value
      return serializeEditor.getApi(MarkdownPlugin).markdown.serialize()
    },
  }
}

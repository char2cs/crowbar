// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import type { Value } from 'platejs'
import { createMarkdownCodec } from './markdown-codec'
import { markdownPlugins } from './markdown-plugins'

// The FILE editor's codec — see markdown-codec.ts for why the headless editors
// behind it are lazy, reused, and one per direction. The comment editor has its
// own (comment/comment-serialization.ts) over a much smaller plugin set.
const codec = createMarkdownCodec(markdownPlugins)

/** Markdown text -> Plate value. */
export function markdownToPlateValue(md: string): Value {
  return codec.toValue(md)
}

/** Plate value -> markdown text. */
export function plateValueToMarkdown(value: Value): string {
  return codec.toMarkdown(value)
}

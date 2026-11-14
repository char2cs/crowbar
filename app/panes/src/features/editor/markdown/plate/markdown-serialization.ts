// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import type { Value } from 'platejs'
import { createMarkdownCodec, type ParsedMarkdown } from './markdown-codec'
import type { MarkdownSource } from './markdown-source'
import { markdownPlugins } from './markdown-plugins'

// The FILE editor's codec — see markdown-codec.ts for why the headless editors
// behind it are lazy, reused, and one per direction. The comment editor has its
// own (comment/comment-serialization.ts) over a much smaller plugin set.
const codec = createMarkdownCodec(markdownPlugins)

/** Markdown text -> Plate value, plus the source map a save writes back over. */
export function parseMarkdown(md: string): ParsedMarkdown {
  return codec.parse(md)
}

/** Plate value -> markdown text; with `source`, untouched blocks keep their exact bytes. */
export function plateValueToMarkdown(value: Value, source?: MarkdownSource): string {
  return codec.toMarkdown(value, source)
}

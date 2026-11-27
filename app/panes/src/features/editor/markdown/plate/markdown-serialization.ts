import type { Value } from 'platejs'
import { createMarkdownCodec, type ParsedMarkdown } from './markdown-codec'
import type { MarkdownSource } from './markdown-source'
import { markdownPlugins } from './markdown-plugins'

const codec = createMarkdownCodec(markdownPlugins)

export function parseMarkdown(md: string): ParsedMarkdown {
  return codec.parse(md)
}

export function plateValueToMarkdown(value: Value, source?: MarkdownSource): string {
  return codec.toMarkdown(value, source)
}

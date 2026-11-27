import type { Value } from 'platejs'
import { createMarkdownCodec } from '../markdown-codec'
import { commentPlugins } from './comment-plugins'

const codec = createMarkdownCodec(commentPlugins)

export function commentMarkdownToValue(md: string): Value {
  return codec.toValue(md)
}

export function commentValueToMarkdown(value: Value): string {
  return codec.toMarkdown(value)
}

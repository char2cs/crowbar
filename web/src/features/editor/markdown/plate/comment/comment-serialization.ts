import type { Value } from 'platejs'
import { createMarkdownCodec } from '../markdown-codec'
import { commentPlugins } from './comment-plugins'

// Separate from the file editor's codec because it is bound to a different
// plugin set, and a plugin set decides which nodes survive a round trip. See
// markdown-codec.ts.
const codec = createMarkdownCodec(commentPlugins)

/** A comment's stored markdown -> the Plate document the editor holds. */
export function commentMarkdownToValue(md: string): Value {
  return codec.toValue(md)
}

/** The Plate document -> the markdown that gets stored on the thread. */
export function commentValueToMarkdown(value: Value): string {
  return codec.toMarkdown(value)
}

import type { Value } from 'platejs'
import { createMarkdownCodec } from '@/features/editor/markdown/plate/markdown-codec'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'

// Its own codec: the plugin set decides which nodes survive a round trip.
const codec = createMarkdownCodec(chatComposerPlugins)

export function chatMarkdownToValue(md: string): Value {
  return codec.toValue(md)
}

type Node = { type?: string; text?: string; code?: boolean; children?: Node[] }

/** Blocks whose whitespace is CONTENT. A code line's indent is the program. */
const VERBATIM = new Set(['code_block', 'code_line'])

function clone(node: Node): Node {
  return node.children ? { ...node, children: node.children.map(clone) } : { ...node }
}

function leavesOf(nodes: Node[], into: Node[] = []): Node[] {
  for (const node of nodes) {
    if (typeof node.text === 'string') into.push(node)
    else if (node.children) leavesOf(node.children, into)
  }
  return into
}

function trimLineEdges(node: Node): Node {
  if (node.type && VERBATIM.has(node.type)) return node
  const children = node.children
  if (!children) return node
  // Slate keeps a text node beside every inline, so prose always has a direct leaf.
  if (!children.some((child) => typeof child.text === 'string')) {
    return { ...node, children: children.map(trimLineEdges) }
  }
  const copied = children.map(clone)
  const leaves = leavesOf(copied)
  for (const leaf of leaves) {
    if (!leaf.code) leaf.text = leaf.text?.replace(/[ \t]*\n[ \t]*/g, '\n')
  }
  const first = leaves[0]
  const last = leaves.at(-1)
  if (first && !first.code) first.text = first.text?.replace(/^[ \t]+/, '')
  if (last && !last.code) last.text = last.text?.replace(/[ \t]+$/, '')
  return { ...node, children: copied }
}

/** Line edges are trimmed on the nodes, since remark-stringify encodes them as `&#x20;`.
 *  U+200B is the codec's empty-paragraph placeholder, which `.trim()` won't strip. */
const ZERO_WIDTH_SPACE = /\u200B/g

export function chatValueToMarkdown(value: Value): string {
  return codec
    .toMarkdown(value.map((node) => trimLineEdges(node as Node)) as Value)
    .replace(ZERO_WIDTH_SPACE, '')
    .trim()
}

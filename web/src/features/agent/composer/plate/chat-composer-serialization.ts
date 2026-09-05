import type { Value } from 'platejs'
import { createMarkdownCodec } from '@/features/editor/markdown/plate/markdown-codec'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'

// Its own codec because it is bound to its own plugin set, and a plugin set is
// what decides which nodes survive a round trip. See markdown-codec.ts for why
// the headless editors behind it are lazy and one per direction.
const codec = createMarkdownCodec(chatComposerPlugins)

/** Markdown a prompt is being resumed from -> the document the box holds. */
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
  // Slate keeps a text node on either side of every inline, so a block that
  // holds prose always has a DIRECT leaf. Anything else is a block container.
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

/**
 * The document -> the markdown that is actually sent to the agent.
 *
 * `remark-stringify` PRESERVES whitespace at the edge of a LINE by encoding it
 * as a character reference, because there it would otherwise mean indentation
 * or a hard break. A prompt typed with a trailing space — most of them, people
 * hit space before Enter — therefore reached the model as `…with Codex?&#x20;`,
 * and a two-line one as `hello&#x20;\n\nworld`. Trimming the finished string
 * only ever reached the document's two outer ends; the edges are per line, so
 * they are cleared on the nodes, before anything can encode them. Clones on the
 * way: this runs over the live editor's own children.
 *
 * REGRESSION: a genuinely empty document — a fresh box, or one typed into and
 * cleared back out — normalizes (Slate cannot hold zero children) to a single
 * empty paragraph, which the codec serializes as a lone U+200B: its own
 * placeholder for telling an intentionally-blank paragraph apart from no
 * paragraph at all on a round trip. `.trim()` does not touch it — U+200B is a
 * formatting character, not whitespace — so a box that read as completely
 * empty could be sent as a real prompt. Stripped before the trim, since it is
 * never something a person meant to send.
 */
const ZERO_WIDTH_SPACE = /\u200B/g

export function chatValueToMarkdown(value: Value): string {
  return codec
    .toMarkdown(value.map((node) => trimLineEdges(node as Node)) as Value)
    .replace(ZERO_WIDTH_SPACE, '')
    .trim()
}

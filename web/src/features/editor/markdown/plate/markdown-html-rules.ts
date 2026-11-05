import type { MdRules } from '@platejs/markdown'

// Raw HTML blocks round-trip as a void Plate `html` element carrying the raw string (rendered
// sanitized by html-node.tsx); the default turns them into escaped text that a save mangles.
// remark-stringify emits `html` nodes verbatim, so the bytes come back exactly.
export const htmlMarkdownRules: MdRules = {
  html: {
    deserialize(mdastNode) {
      return {
        type: 'html',
        html: (mdastNode as { value?: string }).value ?? '',
        children: [{ text: '' }],
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the rule's element shape is app-defined (html-node.tsx), wider than the lib's generic node type
      } as any
    },
    serialize(node) {
      return {
        type: 'html',
        value: (node as { html?: string }).html ?? '',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- returns an mdast `html` node, narrower than the lib's MdRootContent union
      } as any
    },
  },
}

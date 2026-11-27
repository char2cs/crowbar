import type { MdRules } from '@platejs/markdown'

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

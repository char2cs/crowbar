import type { MdRules } from '@platejs/markdown'

export const underlineMarkdownRules: MdRules = {
  underline: {
    mark: true,
    serialize(node) {
      return {
        type: 'html',
        value: `<u>${(node as { text?: string }).text ?? ''}</u>`,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- returns an mdast `html` node, narrower than the lib's MdRootContent union
      } as any
    },
  },
}

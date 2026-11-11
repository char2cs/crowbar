import type { MdRules } from '@platejs/markdown'

// The default `underline` rule serializes to an MDX JSX node no stringify extension here handles,
// so it throws on every onChange. A raw `html` node (`<u>…</u>`) is emitted verbatim instead;
// `deserialize` is omitted on purpose, since Plate falls back to its default per field.
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

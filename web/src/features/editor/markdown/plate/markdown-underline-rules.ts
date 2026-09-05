import type { MdRules } from '@platejs/markdown'

/**
 * @platejs/markdown's own default `underline` rule serializes to an mdast
 * `mdxJsxTextElement` node (`{ name: 'u', type: 'mdxJsxTextElement' }`) —
 * MDX's JSX-in-markdown syntax. Neither this app's `remarkStringifyOptions`
 * nor `remarkPlugins` configures an MDX stringify extension anywhere, and
 * `mdast-util-to-markdown` has no built-in handler for that node type: it
 * throws. Since Plate's `onChange` fires on every op (selection moves
 * included, not just text edits) and re-serializes on each one, any document
 * holding underlined text throws on essentially every keystroke — live-
 * reproduced, not theoretical.
 *
 * Serializing to a plain mdast `html` node instead sidesteps the missing MDX
 * extension entirely: `html` nodes need no extension, remark-stringify emits
 * their `value` VERBATIM (see markdown-html-rules.ts, same technique for raw
 * HTML blocks), and `<u>…</u>` is itself valid, readable markdown-with-HTML
 * for something CommonMark has no native syntax for. `deserialize` is left
 * out on purpose — @platejs/markdown falls back to its own default per KEY
 * per FIELD when a field is omitted from a user rule (verified in
 * @platejs/markdown/dist/index.js's `getDeserializerByKey`), so parsing
 * underline text back out of markdown is untouched by this override.
 */
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

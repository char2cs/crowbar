import type { MdRules } from '@platejs/markdown'

/**
 * Route raw HTML blocks through a dedicated `html` node instead of letting
 * @platejs/markdown fold them into text.
 *
 * Without this, remark parses a raw HTML block (`<div>…</div>`) into an mdast
 * `html` node, the default handling turns it into escaped TEXT, and saving
 * mangles it — `<div>` → `\<div>`, spaces → `&#x20;`. Common README headers
 * get corrupted the moment the file is opened and re-saved.
 *
 * The fix is symmetric with how remark itself treats HTML:
 *  - deserialize: mdast `html` node → a void Plate `html` element carrying the
 *    raw string (see `html-node.tsx`, which renders it sanitized).
 *  - serialize: that element → an mdast `html` node whose `value` is the raw
 *    string. remark-stringify emits `html` nodes VERBATIM (that is their whole
 *    purpose), so the bytes round-trip exactly — no escaping.
 *
 * Merged per-key over @platejs/markdown's defaults alongside the callout rules,
 * so no other node type is affected.
 */
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

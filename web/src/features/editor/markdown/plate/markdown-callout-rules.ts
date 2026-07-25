import type { MdRootContent, MdRules } from '@platejs/markdown'
import { convertNodesDeserialize, convertNodesSerialize, defaultRules } from '@platejs/markdown'
import { KEYS, getPluginType } from 'platejs'
import type { TCalloutElement } from 'platejs'

// GitHub's "alert" blockquote convention: a blockquote whose first line is a
// bare `[!TYPE]` marker (https://github.com/orgs/community/discussions/16925),
// e.g.:
//   > [!NOTE]
//   > Something worth calling out.
//
// @platejs/callout's OWN default markdown rule (see @platejs/markdown's
// `defaultRules.callout`) instead round-trips a callout through an MDX
// `<callout>` tag (mdast `mdxJsxFlowElement`). That requires `remark-mdx` in
// the parse pipeline to come back as a callout on the next open — this editor
// doesn't carry that plugin (GFM + math cover everything else it needs), so
// an MDX-tagged callout would silently degrade to inert paragraph text
// (literally the `<callout>` tag as text) on reload: a real fidelity loss.
// GitHub alert syntax is plain blockquote + text, so it parses correctly with
// nothing beyond the `blockquote`/`paragraph` handling remark already gives
// us for free, AND stays readable/renders natively in GitHub, GitLab and most
// static-site generators — the task's "prefer GitHub alert syntax" call.
//
// Trade-off: only the five canonical GitHub types (NOTE/TIP/IMPORTANT/
// WARNING/CAUTION) get special rendering treatment on GitHub itself, but we
// recognize — and losslessly preserve — ANY `[!TOKEN]` marker generically.
// An uncommon custom `variant` still round-trips through this app without
// being dropped; it just renders as an ordinary blockquote on GitHub.

const ALERT_MARKER = /^\[!([A-Za-z][\w-]*)\](?:\n|$)/

interface MdTextNode {
  type: 'text'
  value: string
  [key: string]: unknown
}

interface MdParagraphNode {
  type: 'paragraph'
  children: unknown[]
  [key: string]: unknown
}

function isParagraphNode(node: unknown): node is MdParagraphNode {
  return (
    typeof node === 'object' &&
    node !== null &&
    (node as { type?: unknown }).type === 'paragraph' &&
    Array.isArray((node as { children?: unknown }).children)
  )
}

function isTextNode(node: unknown): node is MdTextNode {
  return (
    typeof node === 'object' &&
    node !== null &&
    (node as { type?: unknown }).type === 'text' &&
    typeof (node as { value?: unknown }).value === 'string'
  )
}

/**
 * Recognizes a GitHub-alert-shaped blockquote and, if found, splits it into
 * the alert's type token (lowercased) and the remaining mdast block content
 * with the marker line stripped out. Returns `undefined` for an ordinary
 * blockquote, which falls through to the default blockquote handling.
 */
function extractAlert(children: unknown[]): { variant: string; rest: unknown[] } | undefined {
  const [first, ...remainder] = children
  if (!isParagraphNode(first)) return undefined
  const [firstText, ...restOfFirstParagraph] = first.children
  if (!isTextNode(firstText)) return undefined
  const match = ALERT_MARKER.exec(firstText.value)
  if (!match) return undefined

  const variant = match[1].toLowerCase()
  const strippedValue = firstText.value.slice(match[0].length)
  const rest: unknown[] = []
  // Only keep the first paragraph if the marker line wasn't the whole of it —
  // a marker alone on its own paragraph (the common case) leaves nothing behind.
  if (strippedValue.length > 0 || restOfFirstParagraph.length > 0) {
    rest.push({
      ...first,
      children:
        strippedValue.length > 0
          ? [{ ...firstText, value: strippedValue }, ...restOfFirstParagraph]
          : restOfFirstParagraph,
    })
  }
  rest.push(...remainder)
  return { variant, rest }
}

/**
 * Custom `blockquote`/`callout` markdown rules, spread into
 * `MarkdownPlugin.configure({ options: { rules: {...} } })` alongside the
 * defaults (Plate merges `rules` per-key over `defaultRules`, so every other
 * node type — headings, lists, tables, math, etc. — is untouched by this).
 */
export const calloutMarkdownRules: MdRules = {
  blockquote: {
    deserialize(mdastNode, deco, options) {
      const alert = extractAlert((mdastNode as { children: unknown[] }).children)
      if (!alert) {
        // Not a GitHub alert — defer to Plate's own blockquote handling
        // verbatim so ordinary blockquotes are completely unaffected.
        return defaultRules.blockquote!.deserialize!(mdastNode, deco, options)
      }
      return {
        // `options.editor` is typed optional on `DeserializeMdOptions` but is
        // always populated by the time a rule's `deserialize` runs — see
        // `getMergedOptionsDeserialize`, which unconditionally sets it.
        type: getPluginType(options.editor!, KEYS.callout),
        variant: alert.variant,
        children: convertNodesDeserialize(alert.rest as MdRootContent[], deco, options),
      } as unknown as TCalloutElement
    },
  },
  callout: {
    serialize(node, options) {
      const calloutNode = node as TCalloutElement
      const variant =
        typeof calloutNode.variant === 'string' && calloutNode.variant.trim().length > 0
          ? calloutNode.variant.trim()
          : 'note'
      const markerText = `[!${variant.toUpperCase()}]`
      const bodyChildren = convertNodesSerialize(calloutNode.children, options)
      const [firstBody, ...restBody] = bodyChildren

      // Fold the marker into the first paragraph as a leading soft-break
      // line — matching exactly how `extractAlert` strips it back out — so
      // the blockquote renders as `> [!NOTE]\n> body` with no blank line in
      // between, the canonical GitHub form (and this call's own output is
      // therefore a fixed point: reserializing what this just produced
      // yields byte-identical markdown).
      const children =
        firstBody && isParagraphNode(firstBody)
          ? [
              {
                ...firstBody,
                children: [{ type: 'text', value: `${markerText}\n` }, ...firstBody.children],
              },
              ...restBody,
            ]
          : [
              { type: 'paragraph', children: [{ type: 'text', value: markerText }] },
              ...bodyChildren,
            ]

      return { type: 'blockquote', children } as unknown as ReturnType<
        NonNullable<NonNullable<MdRules['callout']>['serialize']>
      >
    },
  },
}

/**
 * Build-time stand-in for `remark-mdx` (aliased in vite.config.ts).
 *
 * `@platejs/markdown` imports remark-mdx statically only to export an optional
 * `remarkMdx` plugin; Crowbar never enables MDX (markdown-codec deserializes
 * `withoutMdx`, and callouts use GitHub alert blockquotes — see
 * markdown-callout-rules.ts). The real module drags acorn and the MDX
 * micromark extensions (~300 KB gzipped) into the markdown chunk that loads
 * with the first chat. This stub keeps that import resolvable and fails
 * loudly if anything ever does try to enable MDX.
 */
export default function remarkMdx(): never {
  throw new Error('MDX is not supported in Crowbar markdown')
}

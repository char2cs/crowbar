// Build-time stand-in for `remark-mdx` (aliased in vite.config.ts): @platejs/markdown imports it
// only for an optional plugin Crowbar never enables, and the real module adds ~300 KB gzipped to
// the first chat's chunk. Keeps the import resolvable and fails loudly if MDX is ever enabled.
export default function remarkMdx(): never {
  throw new Error('MDX is not supported in Crowbar markdown')
}

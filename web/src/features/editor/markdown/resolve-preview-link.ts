/**
 * Resolve a relative Markdown-preview link to a WORKSPACE-RELATIVE path.
 *
 * Everything the preview hands back to the file layer travels to the daemon's
 * workspace-scoped file routes, and the fs engine's safepath.Resolve rejects an
 * absolute path outright (400, "path escapes the workspace"). So the result is
 * never anchored with a leading '/', and a root-relative href ('/docs/a.md')
 * means "relative to the WORKSPACE root", not the filesystem root — the old
 * `${rootFolderPath}${href}` form pasted the workspace ID on the front, which
 * addressed nothing.
 *
 * `..` segments that would climb above the workspace root are dropped rather
 * than preserved; there is nothing above the root to reach.
 */
export function resolvePreviewLinkPath(href: string, currentFilePath: string): string {
  const hrefWithoutAnchor = href.split('#')[0]
  if (!hrefWithoutAnchor) return currentFilePath

  const isRootRelative = hrefWithoutAnchor.startsWith('/')
  // A file AT the workspace root has no '/' at all — `slice(0, -1)` would eat its
  // last character and invent a sibling directory ('README.m/other.md').
  const lastSlash = currentFilePath.lastIndexOf('/')
  const currentDir = lastSlash === -1 ? '' : currentFilePath.slice(0, lastSlash)
  const combined = isRootRelative ? hrefWithoutAnchor : `${currentDir}/${hrefWithoutAnchor}`

  const resolved: string[] = []
  for (const part of combined.split('/')) {
    if (part === '..') {
      resolved.pop()
    } else if (part !== '.' && part !== '') {
      resolved.push(part)
    }
  }

  return resolved.join('/')
}

import { isSelfLoading } from '@/features/editor/lib/asset-data-url'

/**
 * Loads one local asset reference (as written in the HTML) as a URL the
 * sandboxed preview can fetch — a `data:` URL — or null to leave it as-is.
 */
export type LoadAsset = (reference: string) => Promise<string | null>

// The preview iframe is sandboxed with an opaque origin and loads from srcdoc,
// so a relative `src` resolves against nothing and a path is not a URL the
// frame can fetch. Local references are therefore inlined as data: URLs.
const TAG_PATTERN = /<([a-z][a-z0-9-]*)\b[^>]*>/gi
const URL_ATTRIBUTE_PATTERN = /\b(src|href|poster)=(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/gi
const SRCSET_ATTRIBUTE_PATTERN = /\bsrcset=(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/gi

function escapeHtmlAttribute(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;')
}

/** `href` is a resource only on <link>; on <a> it is navigation and stays. */
function isResourceAttribute(tag: string, attribute: string): boolean {
  return attribute.toLowerCase() !== 'href' || tag.toLowerCase() === 'link'
}

function srcsetUrls(value: string): string[] {
  return value
    .split(',')
    .map((candidate) => candidate.trim().split(/\s+/)[0] ?? '')
    .filter(Boolean)
}

function collectLocalReferences(html: string): Set<string> {
  const references = new Set<string>()
  for (const [tagSource, tag = ''] of html.matchAll(TAG_PATTERN)) {
    for (const [, attribute = '', dq, sq, uq] of tagSource.matchAll(URL_ATTRIBUTE_PATTERN)) {
      const value = dq ?? sq ?? uq ?? ''
      if (isResourceAttribute(tag, attribute) && !isSelfLoading(value)) references.add(value)
    }
    for (const [, dq, sq, uq] of tagSource.matchAll(SRCSET_ATTRIBUTE_PATTERN)) {
      for (const url of srcsetUrls(dq ?? sq ?? uq ?? '')) {
        if (!isSelfLoading(url)) references.add(url)
      }
    }
  }
  return references
}

function rewriteTags(html: string, resolved: Map<string, string>): string {
  return html.replace(TAG_PATTERN, (tagSource: string, tag: string) =>
    tagSource
      .replace(
        URL_ATTRIBUTE_PATTERN,
        (match, attribute: string, dq?: string, sq?: string, uq?: string) => {
          const value = dq ?? sq ?? uq ?? ''
          const url = isResourceAttribute(tag, attribute) ? resolved.get(value) : undefined
          return url ? `${attribute}="${escapeHtmlAttribute(url)}"` : match
        },
      )
      .replace(SRCSET_ATTRIBUTE_PATTERN, (match, dq?: string, sq?: string, uq?: string) => {
        const value = dq ?? sq ?? uq ?? ''
        const rewritten = value
          .split(',')
          .map((candidate) => {
            const [url = '', ...descriptors] = candidate.trim().split(/\s+/)
            return [resolved.get(url) ?? url, ...descriptors].join(' ')
          })
          .join(', ')
        return rewritten === value ? match : `srcset="${escapeHtmlAttribute(rewritten)}"`
      }),
  )
}

/**
 * The preview document for `html`: every local resource reference (img/
 * script/video/audio/source `src`, `poster`, `srcset`, and `<link href>`)
 * inlined via `loadAsset`. References that do not load are left untouched.
 */
export async function buildHtmlPreviewDocument(
  html: string,
  loadAsset: LoadAsset,
): Promise<string> {
  const references = [...collectLocalReferences(html)]
  if (references.length === 0) return html
  const loaded = await Promise.all(
    references.map(async (reference) => [reference, await loadAsset(reference).catch(() => null)]),
  )
  const resolved = new Map(
    loaded.filter((entry): entry is [string, string] => typeof entry[1] === 'string'),
  )
  return resolved.size === 0 ? html : rewriteTags(html, resolved)
}

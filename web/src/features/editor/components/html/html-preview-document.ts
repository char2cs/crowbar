import { isSelfLoading, textFromDataUrl } from '@/features/editor/lib/asset-data-url'

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
const SRCDOC_ATTRIBUTE_PATTERN = /\ssrcdoc(?=[\s=/>])/i

function escapeHtmlAttribute(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;')
}

// A whole document rides in one attribute value: a single pass over one entity
// map, so the quote that would end the attribute is handled by the only step.
const SRCDOC_ENTITIES = new Map([
  ['&', '&amp;'],
  ['"', '&quot;'],
  ['<', '&lt;'],
  ['>', '&gt;'],
])

function escapeSrcdoc(text: string): string {
  return text.replace(/[&"<>]/g, (char) => SRCDOC_ENTITIES.get(char) ?? char)
}

/** `href` is a resource only on <link>; on <a> it is navigation and stays. */
function isResourceAttribute(tag: string, attribute: string): boolean {
  return attribute.toLowerCase() !== 'href' || tag.toLowerCase() === 'link'
}

// A frame's `src` is a navigation, and the desktop shell cancels data:
// navigations in every frame; a text document goes in `srcdoc` instead.
function frameSrcdoc(tagSource: string, tag: string, attribute: string, url: string) {
  const isFrameSrc = tag.toLowerCase() === 'iframe' && attribute.toLowerCase() === 'src'
  if (!isFrameSrc || SRCDOC_ATTRIBUTE_PATTERN.test(tagSource)) return null
  const text = textFromDataUrl(url)
  return text === null ? null : `srcdoc="${escapeSrcdoc(text)}"`
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
          if (!url) return match
          return (
            frameSrcdoc(tagSource, tag, attribute, url) ??
            `${attribute}="${escapeHtmlAttribute(url)}"`
          )
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
 * inlined via `loadAsset`; an iframe's local text document becomes its
 * `srcdoc`. References that do not load are left untouched.
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

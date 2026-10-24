import type { DocumentRef } from '@/lib/api/documents'
import { isSelfLoading, mimeForPath, toDataUrl } from '@/features/editor/lib/asset-data-url'
import { siblingRef } from '@/features/editor/documents/document-ref'

/**
 * Loads one local asset reference (as written in the HTML) as a URL the
 * sandboxed preview can fetch — a `data:` URL — or null to leave it as-is.
 */
export type LoadAsset = (reference: string) => Promise<string | null>

/** A local reference inlined: as a `data:` URL, or a stylesheet link as its CSS. */
type Inlined = { url: string } | { css: string }

// The preview iframe is sandboxed with an opaque origin and loads from srcdoc,
// so a relative `src` resolves against nothing and a path is not a URL the
// frame can fetch. Local references are therefore inlined as data: URLs.
const TAG_PATTERN = /<([a-z][a-z0-9-]*)\b[^>]*>/gi
const URL_ATTRIBUTE_PATTERN = /\b(src|href|poster)=(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/gi
const SRCSET_ATTRIBUTE_PATTERN = /\bsrcset=(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/gi
const STYLESHEET_REL_PATTERN = /\brel=(["']?)stylesheet\1(?=[\s/>])/i
const MEDIA_ATTRIBUTE_PATTERN = /\bmedia=(?:"[^"]*"|'[^']*'|[^\s"'=<>`]+)/i

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

/** A stylesheet `<link>` whose CSS was read, as the `<style>` that applies it. */
function inlineStylesheet(
  tag: string,
  tagSource: string,
  resolved: Map<string, Inlined>,
): string | null {
  if (tag.toLowerCase() !== 'link' || !STYLESHEET_REL_PATTERN.test(tagSource)) return null
  for (const [, attribute = '', dq, sq, uq] of tagSource.matchAll(URL_ATTRIBUTE_PATTERN)) {
    const inlined = attribute.toLowerCase() === 'href' ? resolved.get(dq ?? sq ?? uq ?? '') : null
    if (!inlined || !('css' in inlined)) continue
    const media = MEDIA_ATTRIBUTE_PATTERN.exec(tagSource)?.[0]
    // A `</style` inside the CSS would close the element early.
    const css = inlined.css.replace(/<\/style/gi, '<\\/style')
    return `<style${media ? ` ${media}` : ''}>${css}</style>`
  }
  return null
}

function rewriteTags(html: string, resolved: Map<string, Inlined>): string {
  const urlOf = (value: string) => {
    const inlined = resolved.get(value)
    return inlined && 'url' in inlined ? inlined.url : undefined
  }
  return html.replace(TAG_PATTERN, (tagSource: string, tag: string) => {
    const style = inlineStylesheet(tag, tagSource, resolved)
    if (style) return style
    return tagSource
      .replace(
        URL_ATTRIBUTE_PATTERN,
        (match, attribute: string, dq?: string, sq?: string, uq?: string) => {
          const value = dq ?? sq ?? uq ?? ''
          const url = isResourceAttribute(tag, attribute) ? urlOf(value) : undefined
          return url ? `${attribute}="${escapeHtmlAttribute(url)}"` : match
        },
      )
      .replace(SRCSET_ATTRIBUTE_PATTERN, (match, dq?: string, sq?: string, uq?: string) => {
        const value = dq ?? sq ?? uq ?? ''
        const rewritten = value
          .split(',')
          .map((candidate) => {
            const [url = '', ...descriptors] = candidate.trim().split(/\s+/)
            return [urlOf(url) ?? url, ...descriptors].join(' ')
          })
          .join(', ')
        return rewritten === value ? match : `srcset="${escapeHtmlAttribute(rewritten)}"`
      })
  })
}

/** `html` with every local reference `load` inlines; the rest are left untouched. */
async function inlineReferences(
  html: string,
  load: (reference: string) => Promise<Inlined | null>,
): Promise<string> {
  const references = [...collectLocalReferences(html)]
  const resolved = new Map<string, Inlined>()
  await Promise.all(
    references.map(async (reference) => {
      const inlined = await load(reference).catch(() => null)
      if (inlined) resolved.set(reference, inlined)
    }),
  )
  return resolved.size === 0 ? html : rewriteTags(html, resolved)
}

/**
 * The preview document for `html`: every local resource reference (img/
 * script/video/audio/source `src`, `poster`, `srcset`, and `<link href>`)
 * inlined via `loadAsset`. References that do not load are left untouched.
 */
export function buildHtmlPreviewDocument(html: string, loadAsset: LoadAsset): Promise<string> {
  return inlineReferences(html, async (reference) => {
    const url = await loadAsset(reference)
    return url === null ? null : { url }
  })
}

const HOST_PREVIEW_POLICY = "default-src 'none'; style-src 'unsafe-inline'; img-src data:"
const DOCTYPE_PATTERN = /^\s*<!doctype[^>]*>/i

/** The meta goes after any doctype so the document keeps its standards mode. */
function withHostPreviewPolicy(html: string): string {
  const meta = `<meta http-equiv="Content-Security-Policy" content="${HOST_PREVIEW_POLICY}">`
  const doctype = DOCTYPE_PATTERN.exec(html)?.[0] ?? ''
  return doctype + meta + html.slice(doctype.length)
}

/** The asset `reference` names inside `doc`'s own directory subtree, else null. */
function subtreeRef(doc: DocumentRef, reference: string): DocumentRef | null {
  const ref = siblingRef(doc, reference)
  const dir = doc.path.slice(0, doc.path.lastIndexOf('/') + 1)
  return ref.path.startsWith(dir) ? ref : null
}

/**
 * The preview of `html` for a document outside every workspace, under a policy
 * that loads nothing but inline styles and data images. Only assets in its own
 * directory's subtree are read, and only those the policy lets render:
 * stylesheets inline as `<style>`, images as `data:` URLs.
 */
export async function buildHostPreviewDocument(
  html: string,
  doc: DocumentRef,
  read: (ref: DocumentRef) => Promise<string>,
): Promise<string> {
  const load = async (reference: string): Promise<Inlined | null> => {
    const ref = subtreeRef(doc, reference)
    const mime = ref ? mimeForPath(ref.path) : null
    if (!ref || !mime) return null
    if (mime === 'text/css') return { css: await read(ref) }
    return mime.startsWith('image/') ? { url: toDataUrl(mime, await read(ref)) } : null
  }
  return withHostPreviewPolicy(await inlineReferences(html, load))
}

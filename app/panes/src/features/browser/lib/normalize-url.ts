const EXPLICIT_HTTP = /^https?:\/\//i
const LOCAL_HOST = /^(localhost|127\.0\.0\.1)(:|\/|$)/i
// A dotted host (optionally with port and path); a bare word or a phrase is a search.
const DOTTED_HOST = /^[\w-]+(\.[\w-]+)+(:\d+)?([/?#]\S*)?$/

/** What the address bar's text means as a navigation: null when empty, an
 *  absolute http(s) URL for a host, else a search through `searchTemplate`. */
export function normalizeBrowserUrl(input: string, searchTemplate: string): string | null {
  const text = input.trim()
  if (text === '') return null
  if (EXPLICIT_HTTP.test(text)) return text
  if (LOCAL_HOST.test(text)) return `http://${text}`
  if (DOTTED_HOST.test(text)) return `https://${text}`
  return searchTemplate.replace('%s', encodeURIComponent(text))
}

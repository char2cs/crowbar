export function normalizeSearchText(value: string) {
  return value
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .trim()
}

export function compactSearchText(value: string) {
  return normalizeSearchText(value).replace(/\s+/g, '')
}

export interface SearchCandidateField {
  value: string
  weight?: number
}

function searchTextIncludes(value: string, normalizedQuery: string, compactQuery: string) {
  return (
    normalizeSearchText(value).includes(normalizedQuery) ||
    compactSearchText(value).includes(compactQuery)
  )
}

export function matchesSearchQuery(query: string, candidates: string[]) {
  const normalizedQuery = normalizeSearchText(query)
  if (!normalizedQuery) return true

  const compactQuery = compactSearchText(query)

  return candidates.some((candidate) =>
    searchTextIncludes(candidate, normalizedQuery, compactQuery),
  )
}

/** True when `needle` appears in `haystack` as an (order-preserving) subsequence. */
function isSubsequence(needle: string, haystack: string): boolean {
  let i = 0
  for (let j = 0; j < haystack.length && i < needle.length; j++) {
    if (haystack[j] === needle[i]) i++
  }
  return i === needle.length
}

/**
 * Fuzzy, token-based match: the query is split into whitespace tokens and every
 * token must appear in `text` as a subsequence (after normalization). Tokens can
 * appear in any order and across separators, so `"crowbar wave2"` and
 * `"qvr shell"` both match. Empty/whitespace query matches everything.
 */
export function fuzzyMatch(query: string, text: string): boolean {
  const tokens = normalizeSearchText(query).split(' ').filter(Boolean)
  if (tokens.length === 0) return true
  const haystack = normalizeSearchText(text)
  return tokens.every((token) => isSubsequence(token, haystack))
}

export function scoreSearchQuery(query: string, fields: SearchCandidateField[]) {
  const tokens = normalizeSearchText(query).split(/\s+/).filter(Boolean)

  if (tokens.length === 0) return 0

  let score = 0

  for (const token of tokens) {
    const compactToken = compactSearchText(token)
    let tokenMatched = false

    for (const field of fields) {
      if (!searchTextIncludes(field.value, token, compactToken)) continue
      tokenMatched = true
      score += field.weight ?? 1
    }

    if (!tokenMatched) return 0
  }

  return score
}

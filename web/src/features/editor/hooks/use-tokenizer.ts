import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { type Token } from '../utils/html'
import { calculateEdit, isSimpleEdit } from '../utils/tree-sitter-edit'

export interface ViewportRange {
  startLine: number
  endLine: number
  totalLines: number
}

export interface SyntaxTokenSnapshot {
  bufferId: string
  content: string
  tokens: Token[]
}

const LARGE_FILE_LINE_THRESHOLD = 20000
const LARGE_FILE_RANGE_TOKENIZATION_BUFFER_LINES = 160

export function retargetTokensForContentEdit(
  tokens: Token[],
  oldContent: string,
  newContent: string,
): Token[] {
  if (tokens.length === 0 || oldContent === newContent) {
    return tokens
  }

  if (!isSimpleEdit(oldContent, newContent)) {
    return []
  }

  const edit = calculateEdit(oldContent, newContent)
  if (!edit) {
    return tokens
  }

  const delta = edit.newEndIndex - edit.oldEndIndex
  const nextTokens: Token[] = []

  for (const token of tokens) {
    if (token.end <= edit.startIndex) {
      nextTokens.push(token)
      continue
    }

    if (token.start >= edit.oldEndIndex) {
      nextTokens.push({
        ...token,
        start: token.start + delta,
        end: token.end + delta,
      })
      continue
    }

    const startsBeforeEdit = token.start < edit.startIndex
    const endsAfterEdit = token.end > edit.oldEndIndex

    if (startsBeforeEdit && endsAfterEdit) {
      nextTokens.push({
        ...token,
        end: token.end + delta,
      })
      continue
    }

    if (startsBeforeEdit && token.end > edit.startIndex) {
      nextTokens.push({
        ...token,
        end: edit.startIndex,
      })
      continue
    }

    if (endsAfterEdit && token.start < edit.oldEndIndex) {
      nextTokens.push({
        ...token,
        start: edit.newEndIndex,
        end: token.end + delta,
      })
    }
  }

  return nextTokens.filter(
    (token) => token.start < token.end && token.start >= 0 && token.end <= newContent.length,
  )
}

export function resolveSyntaxTokensForContent({
  tokens,
  tokenizedContent,
  normalizedContent,
  bufferId,
  snapshot,
}: {
  tokens: Token[]
  tokenizedContent: string
  normalizedContent: string
  bufferId?: string
  snapshot?: SyntaxTokenSnapshot | null
}): Token[] {
  let sourceTokens = tokens
  let sourceContent = tokenizedContent

  if (sourceTokens.length === 0 && bufferId && snapshot?.bufferId === bufferId) {
    sourceTokens = snapshot.tokens
    sourceContent = snapshot.content
  }

  if (sourceTokens.length === 0) return []
  if (sourceContent === normalizedContent) return sourceTokens
  if (!sourceContent) return sourceTokens

  const retargetedTokens = retargetTokensForContentEdit(
    sourceTokens,
    sourceContent,
    normalizedContent,
  )

  return retargetedTokens.length > 0 ? retargetedTokens : sourceTokens
}

export function expandTokenizationViewportRange(
  viewportRange: ViewportRange,
  lineCount: number,
): ViewportRange {
  const lastLine = Math.max(lineCount - 1, 0)
  const startLine = Math.max(0, Math.min(viewportRange.startLine, lastLine))
  const endLine = Math.max(startLine, Math.min(viewportRange.endLine, lastLine))
  const bufferLines =
    lineCount >= LARGE_FILE_LINE_THRESHOLD
      ? LARGE_FILE_RANGE_TOKENIZATION_BUFFER_LINES
      : EDITOR_CONSTANTS.VIEWPORT_BUFFER_LINES

  return {
    startLine: Math.max(0, startLine - bufferLines),
    endLine: Math.min(lastLine, endLine + bufferLines),
    totalLines: lineCount,
  }
}

export function mergeTokenizedRange({
  cachedTokens,
  rangeTokens,
  rangeStartOffset,
  rangeEndOffset,
  retainOutsideRange,
}: {
  cachedTokens: Token[]
  rangeTokens: Token[]
  rangeStartOffset: number
  rangeEndOffset: number
  retainOutsideRange: boolean
}): Token[] {
  if (!retainOutsideRange) {
    return [...rangeTokens].sort((a, b) => a.start - b.start)
  }

  const cachedTokensOutsideRange = cachedTokens.filter(
    (token) => token.end <= rangeStartOffset || token.start >= rangeEndOffset,
  )

  return [...cachedTokensOutsideRange, ...rangeTokens].sort((a, b) => a.start - b.start)
}

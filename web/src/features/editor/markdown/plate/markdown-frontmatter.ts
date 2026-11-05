// YAML frontmatter is split off before Plate (a leading `---` parses as a thematic break) and its
// exact bytes re-attached on flush. A delimiter is a line that is `---` (or `...` to close) after
// trailing whitespace; every line ending counts, and a BOM is skipped but kept in `frontmatter`.

const BOM = '﻿'

/** Trailing whitespace, including a stray `\r` from a mixed-EOL document. */
function stripTrailingWhitespace(line: string): string {
  return line.replace(/[ \t\r]+$/, '')
}

function isOpenDelimiterLine(line: string): boolean {
  return stripTrailingWhitespace(line) === '---'
}

function isCloseDelimiterLine(line: string): boolean {
  const trimmed = stripTrailingWhitespace(line)
  return trimmed === '---' || trimmed === '...'
}

/** A line that carries no content — empty, or only spaces/tabs (M2). */
function isBlankLine(line: string): boolean {
  return /^[ \t\r]*$/.test(line)
}

interface Line {
  /** Offset of the line's first character. */
  start: number
  /** Offset just past the line's last character (before its line ending). */
  end: number
  /** Offset of the next line's first character (past the line ending). */
  next: number
}

/** The line starting at `from`, honouring CRLF, LF and CR alike; an unterminated final line
 *  reports `next === end`. */
function lineAt(md: string, from: number): Line {
  const eol = /\r\n|\n|\r/g
  eol.lastIndex = from
  const match = eol.exec(md)
  if (!match) return { start: from, end: md.length, next: md.length }
  return { start: from, end: match.index, next: match.index + match[0].length }
}

/** Splits a leading frontmatter block (a first-line `---` with a closing delimiter below) off
 *  `md`, so `frontmatter + body === md`. Blank lines after the close go into `frontmatter`: the
 *  Plate round trip drops a body's leading blank line. No closing delimiter: no frontmatter. */
export function splitFrontmatter(md: string): { frontmatter: string; body: string } {
  const none = { frontmatter: '', body: md }

  const opening = lineAt(md, md.startsWith(BOM) ? BOM.length : 0)
  if (!isOpenDelimiterLine(md.slice(opening.start, opening.end))) return none
  // An opening delimiter with no line ending after it can't have a close below.
  if (opening.next === opening.end) return none

  let cursor = opening.next
  while (cursor < md.length) {
    const line = lineAt(md, cursor)

    if (isCloseDelimiterLine(md.slice(line.start, line.end))) {
      let blockEnd = line.next

      // Blank lines after the close join frontmatter so `body` starts at content; an
      // unterminated trailing line stays in `body` (it is the last line, not a separator).
      while (blockEnd < md.length) {
        const separator = lineAt(md, blockEnd)
        if (separator.next === separator.end) break
        if (!isBlankLine(md.slice(separator.start, separator.end))) break
        blockEnd = separator.next
      }

      return { frontmatter: md.slice(0, blockEnd), body: md.slice(blockEnd) }
    }

    if (line.next === line.end) break // unterminated last line — no close found
    cursor = line.next
  }

  return none
}

/** Inverse of {@link splitFrontmatter}. Returns `body` unchanged when `frontmatter` is `''`. */
export function joinFrontmatter(frontmatter: string, body: string): string {
  if (frontmatter === '') return body
  return frontmatter + body
}

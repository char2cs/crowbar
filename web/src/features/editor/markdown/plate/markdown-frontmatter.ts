// YAML frontmatter must never reach Plate: markdown parses a leading `---`
// block as a thematic break + setext heading, which destroys it on
// serialize (see the Task 8 spec). So we split it off BEFORE deserializing
// and re-attach the exact original bytes on flush — the frontmatter is never
// parsed or re-emitted by the markdown engine.
//
// A line is a delimiter if, once trailing whitespace is stripped, it is
// exactly `---` (open/close) or `...` (close only, per the YAML doc-end
// marker) — matching the frontmatter convention used by Astro/Hugo/Jekyll.
//
// Detection is deliberately byte-tolerant (I3): real files mix line endings
// (a tool appending LF to a CRLF file, or the reverse) and may carry a UTF-8
// BOM. Sniffing ONE end-of-line sequence from the first line break and using
// it as the only separator — or requiring the first byte to be `-` — makes
// detection miss the block entirely, which then hands the `---` fence to
// Plate and corrupts it on the next save. Every line ending is honoured
// individually, and a leading BOM is skipped for detection but kept in the
// emitted `frontmatter` so the bytes still round-trip exactly.

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

/**
 * The line starting at `from`, honouring CRLF, LF and CR endings alike.
 * A final line with no terminator reports `next === end`.
 */
function lineAt(md: string, from: number): Line {
  const eol = /\r\n|\n|\r/g
  eol.lastIndex = from
  const match = eol.exec(md)
  if (!match) return { start: from, end: md.length, next: md.length }
  return { start: from, end: match.index, next: match.index + match[0].length }
}

/**
 * Splits a leading YAML frontmatter block off of `md`, if present.
 *
 * Recognizes frontmatter ONLY when the document starts with a `---`
 * delimiter line (optionally behind a UTF-8 BOM) AND a matching closing
 * `---`/`...` delimiter line exists somewhere below it. `frontmatter`
 * contains the raw block byte-for-byte, including the BOM, both delimiter
 * lines, the closing line's trailing newline (if the document has one there),
 * and any blank line(s) immediately following the close — so
 * `frontmatter + body === md` always, and `body` always starts at the first
 * line of real content.
 *
 * Blank lines are absorbed into `frontmatter` (rather than left as a leading
 * blank line in `body`) because the Plate markdown round trip
 * (markdownToPlateValue -> plateValueToMarkdown) does not preserve a body's
 * leading blank line — leaving it in `body` would make a save silently drop
 * the frontmatter/body separator on every flush. "Blank" includes
 * whitespace-only lines (M2), which are just as invisible to the round trip.
 *
 * A `---` that isn't the very first line (e.g. a mid-document thematic
 * break) is never treated as frontmatter, and a leading `---` with no
 * closing delimiter falls through to "no frontmatter" rather than
 * swallowing the rest of the document.
 */
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

      // Absorb any blank lines immediately following the closing
      // delimiter into frontmatter, so `body` starts at real content. An
      // unterminated trailing line is left in `body` (it is the document's
      // last line, not a separator).
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

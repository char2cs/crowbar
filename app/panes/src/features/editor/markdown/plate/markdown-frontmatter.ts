const BOM = '﻿'

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

function isBlankLine(line: string): boolean {
  return /^[ \t\r]*$/.test(line)
}

interface Line {
  start: number
  end: number
  next: number
}

function lineAt(md: string, from: number): Line {
  const eol = /\r\n|\n|\r/g
  eol.lastIndex = from
  const match = eol.exec(md)
  if (!match) return { start: from, end: md.length, next: md.length }
  return { start: from, end: match.index, next: match.index + match[0].length }
}

export function splitFrontmatter(md: string): { frontmatter: string; body: string } {
  const none = { frontmatter: '', body: md }

  const opening = lineAt(md, md.startsWith(BOM) ? BOM.length : 0)
  if (!isOpenDelimiterLine(md.slice(opening.start, opening.end))) return none
  if (opening.next === opening.end) return none

  let cursor = opening.next
  while (cursor < md.length) {
    const line = lineAt(md, cursor)

    if (isCloseDelimiterLine(md.slice(line.start, line.end))) {
      let blockEnd = line.next

      while (blockEnd < md.length) {
        const separator = lineAt(md, blockEnd)
        if (separator.next === separator.end) break
        if (!isBlankLine(md.slice(separator.start, separator.end))) break
        blockEnd = separator.next
      }

      return { frontmatter: md.slice(0, blockEnd), body: md.slice(blockEnd) }
    }

    if (line.next === line.end) break
    cursor = line.next
  }

  return none
}

export function joinFrontmatter(frontmatter: string, body: string): string {
  if (frontmatter === '') return body
  return frontmatter + body
}

import { describe, expect, it } from 'vitest'
import {
  joinFrontmatter,
  splitFrontmatter,
} from '@/features/editor/markdown/plate/markdown-frontmatter'
import {
  markdownToPlateValue,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

// The core data-safety invariant: splitting and rejoining must reproduce the
// original bytes exactly, for every fixture below.
function expectRoundTrip(md: string) {
  const { frontmatter, body } = splitFrontmatter(md)
  expect(joinFrontmatter(frontmatter, body)).toBe(md)
}

describe('splitFrontmatter / joinFrontmatter', () => {
  it('round-trips a document with no frontmatter', () => {
    const md = '# Hello\n\nJust a normal document.\n'
    expectRoundTrip(md)
    expect(splitFrontmatter(md)).toEqual({ frontmatter: '', body: md })
  })

  it('round-trips normal frontmatter, absorbing the blank separator line into frontmatter', () => {
    const md = '---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n\n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe(
      '---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n\n',
    )
    expect(body).toBe('# Body\n')
  })

  it('absorbs multiple blank lines after the closing delimiter into frontmatter', () => {
    const md = '---\ntitle: Doc\n---\n\n\n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Doc\n---\n\n\n')
    expect(body).toBe('# Body\n')
  })

  it('round-trips frontmatter containing blank lines and [brackets], with no blank separator after the close', () => {
    const md = '---\ntitle: Test\n\ntags: [a, b, c]\nempty:\n---\nBody text follows immediately.\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Test\n\ntags: [a, b, c]\nempty:\n---\n')
    expect(body).toBe('Body text follows immediately.\n')
  })

  it('does not treat a mid-document --- thematic break as frontmatter', () => {
    const md = '# Title\n\nSome text.\n\n---\n\nMore text after a thematic break.\n'
    expectRoundTrip(md)
    expect(splitFrontmatter(md)).toEqual({ frontmatter: '', body: md })
  })

  it('leaves a mid-body thematic break alone when real frontmatter precedes it', () => {
    const md =
      '---\ntitle: Doc\n---\n\nIntro paragraph.\n\n---\n\nSection after a thematic break.\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Doc\n---\n\n')
    expect(body).toBe('Intro paragraph.\n\n---\n\nSection after a thematic break.\n')
  })

  it('treats a document starting with --- but with no closing delimiter as having no frontmatter', () => {
    const md = '---\ntitle: Unterminated\n\nThis never closes.\n'
    expectRoundTrip(md)
    expect(splitFrontmatter(md)).toEqual({ frontmatter: '', body: md })
  })

  it('accepts a ... closing delimiter', () => {
    const md = '---\ntitle: Doc\n...\nBody.\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Doc\n...\n')
    expect(body).toBe('Body.\n')
  })

  it('round-trips CRLF line endings without corrupting them, absorbing the blank separator', () => {
    const md = '---\r\ntitle: CRLF Doc\r\ntags: [x, y]\r\n---\r\n\r\n# Body\r\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\r\ntitle: CRLF Doc\r\ntags: [x, y]\r\n---\r\n\r\n')
    expect(body).toBe('# Body\r\n')
  })

  // I3 (data safety): a file with MIXED line endings — a `\n` first line and
  // `\r\n` after it — is common when a tool appends to a CRLF file (or vice
  // versa). Sniffing a single EOL from the first line ending and using it as
  // the ONLY separator misses the closing delimiter, so the whole `---` block
  // is fed to Plate, which rewrites it into `***` + a setext heading on save.
  it('detects frontmatter when the document mixes LF and CRLF line endings', () => {
    const md = '---\ntitle: x\r\n---\r\n\r\n# Body\r\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: x\r\n---\r\n\r\n')
    expect(body).toBe('# Body\r\n')
  })

  it('detects frontmatter when the document starts CRLF and continues LF', () => {
    const md = '---\r\ntitle: x\n---\n\n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\r\ntitle: x\n---\n\n')
    expect(body).toBe('# Body\n')
  })

  // I3 (data safety): a UTF-8 BOM ahead of the opening `---` defeats the
  // "first line is exactly ---" predicate. The BOM must stay in the output so
  // the bytes round-trip; it rides along in `frontmatter`.
  it('detects frontmatter behind a UTF-8 BOM and keeps the BOM in the output', () => {
    const md = '﻿---\ntitle: BOM Doc\n---\n\n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('﻿---\ntitle: BOM Doc\n---\n\n')
    expect(body).toBe('# Body\n')
  })

  it('round-trips a BOM document with no frontmatter without losing the BOM', () => {
    const md = '﻿# Hello\n\nNo frontmatter here.\n'
    expectRoundTrip(md)
    expect(splitFrontmatter(md)).toEqual({ frontmatter: '', body: md })
  })

  // M2: only fully-empty lines were absorbed after the closing delimiter, so a
  // whitespace-only separator line stayed in `body` — and the Plate round trip
  // drops a body's leading blank line, silently deleting the separator on the
  // first real save.
  it('absorbs a whitespace-only separator line after the closing delimiter', () => {
    const md = '---\ntitle: Doc\n---\n   \n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Doc\n---\n   \n')
    expect(body).toBe('# Body\n')
  })

  it('absorbs mixed empty and whitespace-only separator lines', () => {
    const md = '---\ntitle: Doc\n---\n\n\t\n  \n# Body\n'
    expectRoundTrip(md)
    const { frontmatter, body } = splitFrontmatter(md)
    expect(frontmatter).toBe('---\ntitle: Doc\n---\n\n\t\n  \n')
    expect(body).toBe('# Body\n')
  })

  it('returns empty frontmatter for an empty document', () => {
    expectRoundTrip('')
    expect(splitFrontmatter('')).toEqual({ frontmatter: '', body: '' })
  })

  it('joinFrontmatter returns body unchanged when frontmatter is empty', () => {
    expect(joinFrontmatter('', 'body only\n')).toBe('body only\n')
  })

  // The regression this fix targets: the Plate round trip drops a body's
  // leading blank line (markdownToPlateValue -> plateValueToMarkdown does not
  // preserve leading blank lines), so if that separator line were left in
  // `body` instead of absorbed into `frontmatter`, a real editor flush would
  // silently rewrite the file with the frontmatter/body separator missing.
  // The body here is already in the serializer's canonical form (no GFM
  // table, which would legitimately re-canonicalize and mask the assertion),
  // so `plateValueToMarkdown(markdownToPlateValue(body))` is expected to
  // return `body` unchanged — meaning the join is byte-exact only if the
  // blank separator line was never part of `body` to begin with.
  it('preserves the frontmatter/body blank separator across a full Plate editor round trip', () => {
    const md =
      '---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n\n' +
      '# Heading\n\nSome **bold** text.\n'
    const { frontmatter, body } = splitFrontmatter(md)
    const roundTrippedBody = plateValueToMarkdown(markdownToPlateValue(body))
    expect(roundTrippedBody).toBe(body)
    expect(joinFrontmatter(frontmatter, roundTrippedBody)).toBe(md)
  })
})

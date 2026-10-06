import { describe, expect, it, vi } from 'vitest'
import type { Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { applyStreamedValue } from '@/features/agent/transcript/plate/streaming-value-patch'
import {
  FRESH_CURSOR,
  parseStreamingMarkdown,
  type StreamCursor,
} from '@/features/agent/transcript/plate/streaming-markdown-parse'

const strip = (v: unknown): unknown =>
  JSON.parse(JSON.stringify(v, (k, val) => (k === 'id' || k === 'listStart' ? undefined : val)))

// The editor merges adjacent text leaves and the raw parse does not; both sides
// go through one scratch editor so only real differences remain.
const scratch = createPlateEditor({ plugins: chatComposerPlugins })
function sameDocument(a: Value, b: Value): { a: unknown; b: unknown } {
  const [rawA, rawB] = [strip(a), strip(b)]
  return JSON.stringify(rawA) === JSON.stringify(rawB)
    ? { a: rawA, b: rawA }
    : { a: settled(a), b: settled(b) }
}

function settled(value: Value): unknown {
  scratch.children = structuredClone(value)
  scratch.tf.normalize({ force: true })
  return strip(scratch.children)
}

function rng(seed: number) {
  let s = seed >>> 0
  return () => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0
    return s / 2 ** 32
  }
}

/** Streams `md` in `chunks`-sized random pieces; every step must equal the full parse. */
function streamAndCompare(md: string, seed: number, maxChunk: number) {
  const random = rng(seed)
  const editor = createPlateEditor({ plugins: chatComposerPlugins })
  let cursor: StreamCursor = FRESH_CURSOR
  let at = 0
  while (at < md.length) {
    at = Math.min(md.length, at + 1 + Math.floor(random() * maxChunk))
    const text = md.slice(0, at)
    const step = parseStreamingMarkdown(text, cursor, editor.children as Value, chatMarkdownToValue)
    cursor = step.cursor
    const { a, b } = sameDocument(step.value, chatMarkdownToValue(text))
    expect(a, `seed ${seed} at ${at}: ${JSON.stringify(text.slice(-60))}`).toEqual(b)
    applyStreamedValue(editor, step.value)
  }
  // A definition arriving late rewrites an earlier block; the patcher never
  // revisits settled blocks, and the settled message re-renders from scratch.
  if (!cursor.disabled) {
    const { a, b } = sameDocument(editor.children as Value, chatMarkdownToValue(md))
    expect(a).toEqual(b)
  }
  return cursor
}

const CORPUS: Record<string, string> = {
  paragraphs:
    'First paragraph.\n\nSecond one with **bold** and `code`.\n\nThird [link](http://x.y) end.\n\nFourth.\n\n',
  headings: '# Title\n\nIntro\n\n## Section\n\ntext\n\n### Sub\n\nmore text here\n\n',
  fenced:
    'Before\n\n```ts\nconst a = 1\n\nconst b = 2\n```\n\nAfter fence\n\n~~~\nraw\n\nstill raw\n~~~\n\nEnd\n\n',
  unterminatedFence: 'Intro\n\nMore\n\n```js\nlet x = 1\n\nlet y = 2\n\nnot closed',
  tightList: 'Lead\n\n- one\n- two\n- three\n\nAfter list\n\n1. a\n2. b\n\nTail\n\n',
  looseList: 'Lead\n\n- one\n\n- two\n\n- three\n\nAfter list\n\nmore\n\n',
  orderedLoose: 'Steps\n\n1. first\n\n2. second\n\n3. third\n\nDone\n\nfinal\n\n',
  listAfterPara: 'intro\n- a\n\n- b\n\nouter\n\nlast\n\n',
  nestedList: 'x\n\n- a\n  - b\n    - c\n\n  more a\n\n- d\n\ny\n\nz\n\n',
  startNumber: 'one\n\n5. five\n6. six\n\ntwo\n\n7. seven\n\nthree\n\n',
  table:
    'Intro\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |\n\nAfter table\n\n| x |\n|---|\n| y |\n\nEnd\n\n',
  quotesAndCallouts:
    'a\n\n> quote line\n> more\n\n> [!NOTE]\n> callout body\n\nb\n\n> [!WARNING]\n> careful\n\nc\n\n',
  html: 'a\n\n<div align="center">\n  <b>hi</b>\n</div>\n\nb\n\ntext <u>inline</u> html\n\nc\n\n',
  htmlComment: 'a\n\n<!-- hidden\n\nstill hidden -->\n\nb\n\nc\n\n',
  htmlPre: 'a\n\n<pre>\nx\n\ny\n</pre>\n\nb\n\nc\n\n',
  refDefs: 'See [foo] and [bar][1].\n\nmiddle\n\n[foo]: http://a.b\n\n[1]: http://c.d\n\nend\n\n',
  footnote: 'Claim[^1].\n\nmiddle\n\n[^1]: the note\n\nend\n\n',
  indentedCode: 'a\n\n    code one\n\n    code two\n\nb\n\nc\n\n',
  partialInline: 'one **bold te',
  partialLink: 'one\n\ntwo [lin',
  partialCode: 'one\n\ntwo `cod',
  partialEmphasis: 'one\n\ntwo *emph',
  thematic: 'a\n\n---\n\nb\n\n***\n\nc\n\n',
  setext: 'Title\n=====\n\nSub\n---\n\nbody\n\nmore\n\n',
  hardBreaks: 'line one  \nline two\\\nline three\n\nnext\n\nlast\n\n',
  crlf: 'a\r\n\r\nb\r\n\r\n```\r\nx\r\n\r\ny\r\n```\r\n\r\nc\r\n\r\n',
  leadingBlank: '\n\nfirst\n\nsecond\n\nthird\n\n',
  mixed:
    '# Plan\n\nIntro text.\n\n1. **Step** one\n   detail\n2. Step two\n\n```sh\nmake\n\nmake test\n```\n\n| k | v |\n|---|---|\n| a | b |\n\n> note\n\n- x\n- y\n\nDone.\n\n',
}

const BACKWARD_REACHING = new Set(['htmlComment', 'htmlPre', 'refDefs', 'footnote'])

describe('parseStreamingMarkdown', () => {
  for (const [name, md] of Object.entries(CORPUS)) {
    it(`equals the full parse at every step: ${name}`, () => {
      const cursors = (
        [
          [1, 1],
          [2, 3],
          [3, 7],
          [4, 25],
          [5, 120],
        ] as const
      ).map(([seed, maxChunk]) => streamAndCompare(md, seed, maxChunk))
      // Constructs that reach backwards must switch the incremental path off.
      expect(cursors.every((c) => c.disabled)).toBe(BACKWARD_REACHING.has(name))
    })
  }

  it(
    'equals the full parse for generated documents on random chunkings',
    { timeout: 120_000 },
    () => {
      const frags = [
        'plain words here',
        'a **bold** and *em* and `code` mix',
        '# Heading',
        '## Sub heading',
        '- item one\n- item two',
        '- loose a\n\n- loose b',
        '1. one\n2. two',
        '3. three\n\n4. four',
        '```\ncode\n\nmore code\n```',
        '```py\nprint(1)',
        '| a | b |\n|---|---|\n| 1 | 2 |',
        '> quote\n> two',
        '> [!TIP]\n> tip',
        '    indented code',
        '- parent\n  - child\n\n  para in parent',
        '<span>x</span> inline',
        '<div>\nblock\n</div>',
        '---',
        'text with [link](http://a.b) and ![img](http://i.png)',
        'Title\n---',
        'trailing spaces  \nnext line',
        '~~~\ntilde\n\nfence\n~~~',
        '[ref]: http://r.s',
        '$$\nmath\n\nmore\n$$',
      ]
      let incremental = 0
      for (let seed = 100; seed < 130; seed++) {
        const random = rng(seed)
        const blocks = Array.from({ length: 14 }, () => frags[Math.floor(random() * frags.length)])
        const end = streamAndCompare(
          blocks.join('\n\n') + '\n\n',
          seed,
          1 + Math.floor(random() * 40),
        )
        if (end.blocks > 0) incremental++
      }
      // Not vacuous: most generated documents really ran the incremental path.
      expect(incremental).toBeGreaterThan(10)
    },
  )

  it('settles blocks, keeps their node identity, and parses only the tail', () => {
    const md = Array.from({ length: 40 }, (_, i) => `Paragraph ${i} with some words.`).join('\n\n')
    const editor = createPlateEditor({ plugins: chatComposerPlugins })
    const parse = vi.fn(chatMarkdownToValue)
    let cursor: StreamCursor = FRESH_CURSOR
    let settled: unknown[] = []
    for (let at = 10; at <= md.length; at += 37) {
      const step = parseStreamingMarkdown(md.slice(0, at), cursor, editor.children as Value, parse)
      cursor = step.cursor
      applyStreamedValue(editor, step.value)
      if (cursor.blocks >= 10 && settled.length === 0) settled = editor.children.slice(0, 10)
    }
    expect(settled).toHaveLength(10)
    settled.forEach((node, i) => expect(editor.children[i]).toBe(node))
    parse.mockClear()
    parseStreamingMarkdown(md + ' more', cursor, editor.children as Value, parse)
    const parsed = parse.mock.calls.map(([text]) => text.length)
    expect(Math.max(...parsed)).toBeLessThan(md.length / 4)
  })

  it('starts over when the text is rewritten rather than extended', () => {
    const editor = createPlateEditor({ plugins: chatComposerPlugins })
    const a = 'one\n\ntwo\n\nthree\n\nfour\n\nfive'
    const first = parseStreamingMarkdown(
      a,
      FRESH_CURSOR,
      editor.children as Value,
      chatMarkdownToValue,
    )
    applyStreamedValue(editor, first.value)
    const b = 'uno\n\ndos\n\ntres'
    const second = parseStreamingMarkdown(
      b,
      first.cursor,
      editor.children as Value,
      chatMarkdownToValue,
    )
    expect(settled(second.value)).toEqual(settled(chatMarkdownToValue(b)))
  })
})

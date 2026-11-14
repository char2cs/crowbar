import type { Value } from 'platejs'

/** Prompts whose markdown escapes matter, as the composer would hold them. */
const p = (text: string) => ({ type: 'p', children: [{ text }] })

const typedTexts = [
  'snake_case [x](y)',
  'rename snake_case_file.py to __init__.py',
  'a * b * c and 2*3*4',
  'use `foo` literally? no: a lone ` backtick',
  'issue #42 and # not a heading here',
  'compare a < b > c and <div> tags & entities',
  'open C:\\path\\to\\file and a\\b',
  '1. not a list when typed mid-paragraph? 1) either',
  '~~not struck~~ and !important and | pipes |',
  '_a_b_',
  '__init__.py',
  'escape \\_ and \\* and \\[',
  '&amp; &lt;b&gt;',
  '---',
  '+ plus',
  '~~~',
  'regex ^\\d+\\.\\s*$',
  'C:\\Users\\me\\_dir',
  'x = a_b * c_d * e',
  '*args and **kwargs',
  'def f(*args, **kwargs)',
  'a <!-- c --> b',
  '1) item',
  '* star bullet',
  '#hashtag',
  '<script>x</script>',
  'tab\tinside',
  'trail\\',
  'line\\\nnext',
  'café ñ 😀 中文',
  '[ref]: http://x',
  '[a][b]',
  'call set_chat_title [now]',
]
export const leaf = (text: string, marks: Record<string, boolean> = {}) => ({ text, ...marks })
const structured: Record<string, Value> = {
  'link text with a bracket': [
    {
      type: 'p',
      children: [leaf('see '), { type: 'a', url: 'http://u', children: [leaf('a]b')] }, leaf('')],
    },
  ],
  'italic ending in a backslash': [
    { type: 'p', children: [leaf('x '), leaf('dir\\', { italic: true }), leaf(' y')] },
  ],
  'a star beside bold': [{ type: 'p', children: [leaf('foo*'), leaf('bar', { bold: true })] }],
  'a backtick beside code': [{ type: 'p', children: [leaf('a`'), leaf('b', { code: true })] }],
  'a list item starting with a dash': [
    { type: 'p', indent: 1, listStyleType: 'disc', children: [leaf('- inner')] },
  ],
  'a heading starting with a hash': [{ type: 'h2', children: [leaf('# hash')] }],
  'a quote starting with >': [{ type: 'blockquote', children: [p('> nested?')] }],
  'an indented code line': [
    {
      type: 'code_block',
      children: [{ type: 'code_line', children: [leaf('    if a_b: x = c*d')] }],
    },
  ],
  'table cells holding | and \\|': [
    {
      type: 'table',
      children: [
        {
          type: 'tr',
          children: [
            { type: 'th', children: [p('h|1')] },
            { type: 'th', children: [p('h2')] },
          ],
        },
        {
          type: 'tr',
          children: [
            { type: 'td', children: [p('a|b')] },
            { type: 'td', children: [p('c\\|d')] },
          ],
        },
      ],
    },
  ],
} as Record<string, Value>

export const promptCases: [string, Value][] = [
  ...typedTexts.map((text): [string, Value] => [JSON.stringify(text), [p(text)] as Value]),
  ...Object.entries(structured),
]

type Doc = { text?: string; children?: Doc[]; [key: string]: unknown }

/** Slate may split or pad text leaves anywhere; joining runs of identical marks
    leaves only what a reader can see. */
export function sameText(nodes: Value): Doc[] {
  const out: Doc[] = []
  for (const node of nodes as Doc[]) {
    if (node.children) {
      out.push({ ...node, children: sameText(node.children as Value) })
      continue
    }
    const { text = '', ...marks } = node
    const prev = out.at(-1)
    const prevMarks = prev && prev.text !== undefined ? { ...prev, text: undefined } : undefined
    if (
      prev &&
      prevMarks &&
      JSON.stringify(prevMarks) === JSON.stringify({ ...marks, text: undefined })
    )
      prev.text += text
    else if (text !== '') out.push({ ...node })
  }
  return out
}

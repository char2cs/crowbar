// Totals, not per-file counts, so moving code between files does not churn the baseline.
// The patterns are approximate by design: a `catch` inside a string literal would count.
import { realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { countLines, loadCode, runGate } from './lib.mjs'

export const COUNTERS = [
  { id: 'go.timers', lang: 'go', re: /\btime\.(?:After|NewTimer|NewTicker|AfterFunc|Tick)\(/g },
  // Single space after `_` on purpose: gofmt-aligned `_        = T{}` is a compile-time assertion, not a discard.
  { id: 'go.discarded-errors', lang: 'go', re: /^[ \t]*_(?:, _)* = /gm },
  { id: 'web.timers', lang: 'web', re: /\b(?:setTimeout|setInterval)\b/g },
  // Whitespace and comments alone make a body empty, so `// ignore` does not hide a swallow.
  { id: 'web.empty-catch', lang: 'web', find: emptyCatches },
  { id: 'web.swallowed-promise', lang: 'web', find: swallowedPromises },
]

/** Every match of `counter` in the non-test files of its language. */
function* scan(files, { lang, re, find }) {
  for (const file of files) {
    if (file.test || file.lang !== lang) continue
    if (find) for (const index of find(file.text)) yield { file, index }
    else for (const match of file.text.matchAll(re)) yield { file, index: match.index }
  }
}

// The web counters were nested-quantifier regexes, exponential on an unclosed body. These
// scanners keep those regexes' exact matches, backtracking order included, in linear time;
// a body is /(?:\s|\/\/[^\n]*|\/\*[\s\S]*?\*\/)*\}/ and the rest is spelled out inline.
const FAIL = -1
/** A lookup of the UTF-16 units `re` matches: the regex's own class, at an array index's cost. */
function charClass(re) {
  const table = new Uint8Array(0x10000)
  for (let c = 0; c < table.length; c++) table[c] = re.test(String.fromCharCode(c)) ? 1 : 0
  return (ch) => ch !== undefined && table[ch.charCodeAt(0)] === 1
}
const isSpace = charClass(/\s/)
const isWord = charClass(/\w/)
const isIdent = charClass(/[\w$]/)
const isDigit = (c) => c >= '0' && c <= '9'

/** at[i] = the first j >= i whose char satisfies `pred`, or text.length. */
function firstFrom(text, pred) {
  const at = new Int32Array(text.length + 2).fill(text.length)
  for (let i = text.length - 1; i >= 0; i--) at[i] = pred(text[i]) ? i : at[i + 1]
  return at
}

/**
 * end[i] = where an empty body whose loop is at i ends, `then` included, or FAIL. Each i takes
 * what a backtracking engine tries first: a space, the longest `//` run that still ends, the
 * shortest block comment that does, then the `}`. Right to left, so every lookup is ready.
 */
function emptyBodyEnds(text, then) {
  const n = text.length
  const end = new Int32Array(n + 3).fill(FAIL)
  const lastOnLine = new Int32Array(n + 3).fill(FAIL) // the last j in i..end of line with end[j]
  const afterClose = new Int32Array(n + 3).fill(FAIL) // end[j + 2] for the first `*/` at j >= i that ends
  for (let i = n; i >= 0; i--) {
    let r = FAIL
    if (isSpace(text[i])) r = end[i + 1]
    if (r === FAIL && text.startsWith('//', i) && lastOnLine[i + 2] !== FAIL) r = end[lastOnLine[i + 2]]
    if (r === FAIL && text.startsWith('/*', i)) r = afterClose[i + 2]
    if (r === FAIL && text[i] === '}') r = then(i + 1)
    end[i] = r
    const lineGoesOn = i < n && text[i] !== '\n'
    lastOnLine[i] = lineGoesOn && lastOnLine[i + 1] !== FAIL ? lastOnLine[i + 1] : r !== FAIL ? i : FAIL
    afterClose[i] = text.startsWith('*/', i) && end[i + 2] !== FAIL ? end[i + 2] : afterClose[i + 1]
  }
  return end
}

/** Index of every `catch` whose block holds only whitespace and comments. */
function* emptyCatches(text) {
  let next, ends
  for (let at = text.indexOf('catch'); at !== -1; ) {
    let from = at + 1
    if (!isWord(text[at - 1]) && !isWord(text[at + 5])) {
      next ??= { space: firstFrom(text, (c) => !isSpace(c)), close: firstFrom(text, (c) => c === ')') }
      let p = next.space[at + 5]
      if (text[p] === '(') p = next.close[p + 1] < text.length ? next.space[next.close[p + 1] + 1] : text.length
      if (text[p] === '{') ends ??= emptyBodyEnds(text, (i) => i)
      if (text[p] === '{' && ends[p + 1] !== FAIL) {
        yield at
        from = ends[p + 1]
      }
    }
    at = text.indexOf('catch', from)
  }
}

/** Index of every `.catch(` whose handler drops the rejection: an empty body or a bare constant. */
function* swallowedPromises(text) {
  let s
  for (let at = text.indexOf('.catch('); at !== -1; ) {
    s ??= promiseScanner(text)
    const end = s.handlerEnd(s.space[at + 7])
    if (end !== FAIL) yield at
    at = text.indexOf('.catch(', end !== FAIL ? end : at + 1)
  }
}

function promiseScanner(text) {
  const n = text.length
  const space = firstFrom(text, (c) => !isSpace(c))
  const notIdent = firstFrom(text, (c) => !isIdent(c))
  const notDigit = firstFrom(text, (c) => !isDigit(c))
  const close = firstFrom(text, (c) => c === ')')
  const quoteEnd = { "'": firstFrom(text, (c) => c === "'" || c === '\n'), '"': firstFrom(text, (c) => c === '"' || c === '\n') }
  const closed = (i) => (text[space[i]] === ')' ? space[i] + 1 : FAIL) // \s*\)
  const bodies = emptyBodyEnds(text, closed)
  const afterParens = (i) => (text[i] === '(' && close[i + 1] < n ? close[i + 1] + 1 : FAIL) // \([^)]*\)
  const quoted = (i) => {
    const to = quoteEnd[text[i]][i + 1]
    return text[to] === text[i] ? closed(to + 1) : FAIL
  }
  const number = (i) => {
    const d = text[i] === '-' ? i + 1 : i
    if (!isDigit(text[d])) return FAIL
    const whole = notDigit[d]
    return closed(text[whole] === '.' && isDigit(text[whole + 1]) ? notDigit[whole + 1] : whole)
  }
  const value = (i) => {
    if (text[i] === '{') return bodies[i + 1]
    if (text[i] === "'" || text[i] === '"') return quoted(i)
    for (const word of ['undefined', 'null', 'void 0', 'true', 'false']) if (text.startsWith(word, i)) return closed(i + word.length)
    return number(i)
  }
  const arrow = (i) => {
    const params = text[i] === '(' ? afterParens(i) : notIdent[i] > i ? notIdent[i] : FAIL
    if (params === FAIL || !text.startsWith('=>', space[params])) return FAIL
    return value(space[space[params] + 2])
  }
  const fn = (i) => {
    if (!text.startsWith('function', i)) return FAIL
    const params = afterParens(space[notIdent[space[i + 8]]])
    if (params === FAIL || text[space[params]] !== '{') return FAIL
    return bodies[space[params] + 1]
  }
  return { space, handlerEnd: (i) => (arrow(i) !== FAIL ? arrow(i) : fn(i)) }
}

/** id -> match count over non-test files of the counter's language; every id is present. */
export function measureCounters(files) {
  const out = {}
  for (const counter of COUNTERS) out[counter.id] = [...scan(files, counter)].length
  return out
}

/** `path:line` of every match of counter `id`, for finding what to remove. */
export function listMatches(files, id) {
  const counter = COUNTERS.find((c) => c.id === id)
  return [...scan(files, counter)].map(({ file, index }) => `${file.path}:${countLines(file.text.slice(0, index)) + 1}`)
}

const dir = dirname(fileURLToPath(import.meta.url))

export function main(args, { files, baselinePath = join(dir, 'counters-baseline.json'), log = console.log } = {}) {
  const listAt = args.indexOf('--list')
  if (listAt !== -1) {
    const id = args[listAt + 1]
    if (!COUNTERS.some((c) => c.id === id)) {
      log(`unknown counter ${id ?? '(none given)'}; valid ids: ${COUNTERS.map((c) => c.id).join(', ')}`)
      return 1
    }
    for (const hit of listMatches(files ?? loadCode(), id)) log(hit)
    return 0
  }
  const measured = measureCounters(files ?? loadCode())
  return runGate({
    name: 'counters',
    baselinePath,
    measured,
    init: args.includes('--init'),
    update: args.includes('--update'),
    summary: Object.entries(measured).map(([id, n]) => `${id} ${n}`).join(', '),
    hint: 'Remove the pattern, or handle the case it hides. Find the matches: node scripts/ratchet/counters.mjs --list <id>',
    log,
  })
}

// realpath: node resolves the module URL through symlinks but not argv[1]; a mismatch would skip main and exit 0.
if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) process.exit(main(process.argv.slice(2)))

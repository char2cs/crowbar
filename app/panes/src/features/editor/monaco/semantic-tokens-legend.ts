import type { SyntaxTokenKey } from '@/features/editor/theme/resolve-css-color'
import type { ShikiToken } from './shiki-tokens'

const SEMANTIC_TOKEN_TYPES = [
  'namespace',
  'type',
  'class',
  'enum',
  'interface',
  'struct',
  'typeParameter',
  'parameter',
  'variable',
  'property',
  'enumMember',
  'event',
  'function',
  'method',
  'macro',
  'keyword',
  'modifier',
  'comment',
  'string',
  'number',
  'regexp',
  'operator',
  'decorator',
  'label',
  'tag',
  'attribute',
] as const
type SemanticTokenType = (typeof SEMANTIC_TOKEN_TYPES)[number]

const SEMANTIC_TOKEN_MODIFIERS = [
  'readonly',
  'declaration',
  'definition',
  'static',
  'deprecated',
  'abstract',
  'async',
  'modification',
  'documentation',
  'defaultLibrary',
] as const

export const SEMANTIC_TOKEN_LEGEND = {
  tokenTypes: [...SEMANTIC_TOKEN_TYPES] as string[],
  tokenModifiers: [...SEMANTIC_TOKEN_MODIFIERS] as string[],
}

const TYPE_PALETTE: Record<SemanticTokenType, SyntaxTokenKey> = {
  namespace: 'type',
  type: 'type',
  class: 'type',
  enum: 'type',
  interface: 'type',
  struct: 'type',
  typeParameter: 'type',
  parameter: 'variable',
  variable: 'variable',
  property: 'property',
  enumMember: 'constant',
  event: 'property',
  function: 'function',
  method: 'function',
  macro: 'function',
  keyword: 'keyword',
  modifier: 'keyword',
  comment: 'comment',
  string: 'string',
  number: 'number',
  regexp: 'regex',
  operator: 'operator',
  decorator: 'attribute',
  label: 'constant',
  tag: 'tag',
  attribute: 'attribute',
}

export const SEMANTIC_TOKEN_RULES: ReadonlyArray<[selector: string, key: SyntaxTokenKey]> = [
  ...SEMANTIC_TOKEN_TYPES.map((type): [string, SyntaxTokenKey] => [type, TYPE_PALETTE[type]]),
  ['variable.readonly', 'constant'],
]

const typeIndex = (type: SemanticTokenType) => SEMANTIC_TOKEN_TYPES.indexOf(type)
const READONLY = 1 << SEMANTIC_TOKEN_MODIFIERS.indexOf('readonly')

const SHIKI_ENCODING: Record<ShikiToken['category'], [number, number]> = {
  function: [typeIndex('function'), 0],
  type: [typeIndex('type'), 0],
  variable: [typeIndex('variable'), 0],
  property: [typeIndex('property'), 0],
  constant: [typeIndex('variable'), READONLY],
  tag: [typeIndex('tag'), 0],
  attribute: [typeIndex('attribute'), 0],
  keyword: [typeIndex('keyword'), 0],
  string: [typeIndex('string'), 0],
  comment: [typeIndex('comment'), 0],
  number: [typeIndex('number'), 0],
  operator: [typeIndex('operator'), 0],
}

export function encodeShikiTokens(tokens: readonly ShikiToken[]): Uint32Array {
  const data = new Uint32Array(tokens.length * 5)
  let prevRow = 0
  let prevStart = 0
  let i = 0
  for (const t of tokens) {
    const [type, modifiers] = SHIKI_ENCODING[t.category]
    const deltaLine = t.row - prevRow
    data[i++] = deltaLine
    data[i++] = deltaLine === 0 ? t.start - prevStart : t.start
    data[i++] = t.end - t.start
    data[i++] = type
    data[i++] = modifiers
    prevRow = t.row
    prevStart = t.start
  }
  return data
}

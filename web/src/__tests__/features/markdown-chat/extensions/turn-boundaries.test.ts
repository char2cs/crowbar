import { Text } from '@codemirror/state'
import { parseTurnBoundaries, turnsToDocument } from '@/features/markdown-chat/extensions/turn-boundaries'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

const TURNS: MarkdownTurn[] = [
  { id: 'a1', role: 'agent', content: 'Hello', timestamp: '', authorName: 'Claude', widgets: [] },
  { id: 'u1', role: 'user', content: 'World', timestamp: '', authorName: 'Mateo', widgets: [] },
]

test('turnsToDocument produces boundary markers followed by content', () => {
  const doc = turnsToDocument(TURNS)
  expect(doc).toContain('<!-- turn:a1 role:agent -->')
  expect(doc).toContain('Hello')
  expect(doc).toContain('<!-- turn:u1 role:user -->')
  expect(doc).toContain('World')
  expect(doc).not.toContain('<!-- input -->')
})

test('parseTurnBoundaries finds turn ranges in document text', () => {
  const doc = turnsToDocument(TURNS)
  const text = Text.of(doc.split('\n'))
  const ranges = parseTurnBoundaries(text)
  expect(ranges).toHaveLength(2)
  expect(ranges[0].id).toBe('a1')
  expect(ranges[0].role).toBe('agent')
  expect(ranges[1].id).toBe('u1')
  expect(ranges[1].role).toBe('user')
})

test('parseTurnBoundaries returns correct from/to for each turn', () => {
  const doc = turnsToDocument(TURNS)
  const text = Text.of(doc.split('\n'))
  const ranges = parseTurnBoundaries(text)
  // The first turn starts at position 0
  expect(ranges[0].from).toBe(0)
  // The second turn starts after the first turn's content
  expect(ranges[1].from).toBeGreaterThan(ranges[0].from)
  // to positions are monotonically increasing
  expect(ranges[1].to).toBeGreaterThan(ranges[0].to)
})

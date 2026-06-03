import { test, expect } from 'vitest'
import { createMockChat } from '@/lib/mock/chats'

test('two rapid createMockChat calls produce unique IDs', () => {
  const c1 = createMockChat()
  const c2 = createMockChat()
  expect(c1.id).not.toBe(c2.id)
})

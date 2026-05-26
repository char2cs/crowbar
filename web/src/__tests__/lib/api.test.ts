import { test, expect } from 'vitest'
import * as api from '@/lib/api'

test('api module does not export apiFetch (use transport.ts instead)', () => {
  expect((api as Record<string, unknown>).apiFetch).toBeUndefined()
})

test('IS_MOCK is exported and true in test environment (no VITE_API_URL set)', () => {
  expect(api.IS_MOCK).toBe(true)
})

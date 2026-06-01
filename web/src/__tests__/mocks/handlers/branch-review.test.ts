import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest'
import { setupServer } from 'msw/node'
import { branchReviewHandlers } from '@/mocks/handlers/branch-review'
import { fsHandlers } from '@/mocks/handlers/fs'

const server = setupServer(...branchReviewHandlers, ...fsHandlers)
beforeAll(() => server.listen())
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('branch-review handlers', () => {
  it('GET /api/v0/branch-review/:wsId/diff returns a MultiFileDiff', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/diff')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(data).toHaveProperty('files')
    expect(data).toHaveProperty('totalFiles')
    expect(Array.isArray(data.files)).toBe(true)
  })

  it('GET /api/v0/branch-review/:wsId/threads returns an array', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/threads')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(Array.isArray(data)).toBe(true)
  })

  it('GET /api/v0/branch-review/:wsId/description returns a string', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/description')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(typeof data).toBe('string')
  })

  it('GET /api/v0/branch-review/:wsId/chats returns an array', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/chats')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(Array.isArray(data)).toBe(true)
  })

  it('GET /api/v0/fs/file returns file content string', async () => {
    const res = await fetch('/api/v0/fs/file?path=api/go.mod')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(typeof data).toBe('string')
  })
})

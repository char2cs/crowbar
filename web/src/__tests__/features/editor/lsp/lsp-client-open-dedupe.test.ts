import { describe, it, expect, vi, beforeEach } from 'vitest'

// I4 regression: the retained-editor path has TWO independent owners that each
// open/close the same managed file (the satellite hook for diagnostics, and
// useLspIntegration for server start + rich services). The LspClient must emit
// exactly ONE `/didOpen` per file and `/didClose` only when the LAST holder
// closes — otherwise the backend sees duplicate didOpen notifications.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]) => ({ ok: true })),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-1',
}))

import { LspClient } from '@/features/editor/lsp/lsp-client'

function postPaths() {
  return apiFetch.mock.calls.map((c) => String(c[0]))
}

describe('LspClient didOpen/didClose dedupe (I4)', () => {
  beforeEach(() => {
    apiFetch.mockClear()
  })

  it('opens a document exactly once even when two owners open it', async () => {
    const client = new LspClient()
    await client.documentOpen('/src/a.ts', 'code', 'typescript') // owner 1
    await client.documentOpen('/src/a.ts', 'code', 'typescript') // owner 2 (dup)

    const opens = postPaths().filter((p) => p.endsWith('/didOpen'))
    expect(opens).toHaveLength(1)
  })

  it('closes only when the LAST holder closes (refcounted)', async () => {
    const client = new LspClient()
    await client.documentOpen('/src/a.ts', 'code', 'typescript')
    await client.documentOpen('/src/a.ts', 'code', 'typescript')
    apiFetch.mockClear()

    await client.documentClose('/src/a.ts') // one holder gone — no didClose yet
    expect(postPaths().filter((p) => p.endsWith('/didClose'))).toHaveLength(0)

    await client.documentClose('/src/a.ts') // last holder — didClose fires once
    expect(postPaths().filter((p) => p.endsWith('/didClose'))).toHaveLength(1)
  })

  it('a reopen after the final close emits didOpen again', async () => {
    const client = new LspClient()
    await client.documentOpen('/src/a.ts', 'code', 'typescript')
    await client.documentClose('/src/a.ts')
    apiFetch.mockClear()

    await client.documentOpen('/src/a.ts', 'code', 'typescript')
    expect(postPaths().filter((p) => p.endsWith('/didOpen'))).toHaveLength(1)
  })

  it('a close for a never-opened file is a no-op', async () => {
    const client = new LspClient()
    await client.documentClose('/never.ts')
    expect(postPaths().filter((p) => p.endsWith('/didClose'))).toHaveLength(0)
  })
})

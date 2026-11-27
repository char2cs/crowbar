import { createElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  loadLocalImage,
  MarkdownAssetContext,
  useMarkdownAsset,
  type MarkdownAssetInfo,
} from '@/features/editor/markdown/plate/markdown-asset'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'

const { readDisk } = vi.hoisted(() => ({ readDisk: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument: async (ref: DocumentRef) => ({ ref, text: await readDisk(ref) }),
}))

const beside = (path: string): MarkdownAssetInfo => ({ doc: workspaceRef('ws1', path) })

beforeEach(() => {
  readDisk.mockClear()
})

describe('loadLocalImage resolve override', () => {
  it('calls the override with the raw src, bypassing the document read', async () => {
    const resolve = vi.fn().mockResolvedValue('data:image/png;base64,AAAA')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', resolve }

    const signal = new AbortController().signal
    const result = await loadLocalImage(asset, 'chats/c1/attachments/x.png', signal)

    expect(result).toBe('data:image/png;base64,AAAA')
    expect(resolve).toHaveBeenCalledWith('chats/c1/attachments/x.png', signal)
    expect(readDisk).not.toHaveBeenCalled()
  })

  it('still returns null for a self-loading src without calling the override', async () => {
    const resolve = vi.fn()
    const asset: MarkdownAssetInfo = { wsId: 'ws1', resolve }

    expect(await loadLocalImage(asset, 'https://example.com/a.png')).toBeNull()
    expect(resolve).not.toHaveBeenCalled()
  })

  it('falls back to the document read when no override is set (pre-existing behaviour)', async () => {
    readDisk.mockResolvedValue('\x89PNG')
    const result = await loadLocalImage(beside('docs/README.md'), 'logo.png')

    expect(readDisk).toHaveBeenCalledWith(workspaceRef('ws1', 'docs/logo.png'))
    expect(result).toMatch(/^data:image\/png;base64,/)
  })

  it('returns null when there is no asset (pre-existing early return)', async () => {
    expect(await loadLocalImage(null, 'logo.png')).toBeNull()
    expect(readDisk).not.toHaveBeenCalled()
  })

  it('returns null for an empty src (pre-existing early return)', async () => {
    const asset = beside('README.md')
    expect(await loadLocalImage(asset, '')).toBeNull()
    expect(readDisk).not.toHaveBeenCalled()
  })

  it('returns null for a non-image extension (pre-existing early return)', async () => {
    const asset = beside('README.md')
    expect(await loadLocalImage(asset, 'notes.txt')).toBeNull()
    expect(readDisk).not.toHaveBeenCalled()
  })

  it('utf8-encodes an svg read', async () => {
    readDisk.mockResolvedValue('<svg></svg>')
    const asset = beside('README.md')

    const result = await loadLocalImage(asset, 'icon.svg')

    expect(result).toBe(`data:image/svg+xml;charset=utf-8,${encodeURIComponent('<svg></svg>')}`)
  })

  it('returns null when the document read rejects (pre-existing behaviour)', async () => {
    readDisk.mockRejectedValue(new Error('not found'))
    const asset = beside('README.md')

    expect(await loadLocalImage(asset, 'missing.png')).toBeNull()
  })
})

describe('loadLocalImage beside its document', () => {
  it('reads a leading-slash src from the worktree root', async () => {
    readDisk.mockResolvedValue('\x89PNG')
    await loadLocalImage(beside('docs/nested/a.md'), '/assets/logo.png')
    expect(readDisk).toHaveBeenCalledWith(workspaceRef('ws1', 'assets/logo.png'))
  })

  it('reads a host document’s image from its own directory on disk', async () => {
    readDisk.mockResolvedValue('\x89PNG')
    const asset: MarkdownAssetInfo = { doc: { authority: 'host', path: '/tmp/notes/a.md' } }

    expect(await loadLocalImage(asset, '../img/x.png')).toMatch(/^data:image\/png;base64,/)
    expect(readDisk).toHaveBeenCalledWith({ authority: 'host', path: '/tmp/img/x.png' })
  })
})

describe('useMarkdownAsset', () => {
  it('returns null with no provider (pre-existing default)', () => {
    const { result } = renderHook(() => useMarkdownAsset())
    expect(result.current).toBeNull()
  })

  it('returns the provided asset info', () => {
    const asset = beside('docs/README.md')
    const { result } = renderHook(() => useMarkdownAsset(), {
      wrapper: ({ children }) =>
        createElement(MarkdownAssetContext.Provider, { value: asset }, children),
    })
    expect(result.current).toBe(asset)
  })
})

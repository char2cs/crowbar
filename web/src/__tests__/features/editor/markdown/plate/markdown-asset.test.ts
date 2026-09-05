import { createElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  loadLocalImage,
  MarkdownAssetContext,
  resolveAssetPath,
  useMarkdownAsset,
  type MarkdownAssetInfo,
} from '@/features/editor/markdown/plate/markdown-asset'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'

vi.mock('@/features/file-system/controllers/platform', () => ({
  readWorkspaceFile: vi.fn(),
}))

beforeEach(() => {
  vi.mocked(readWorkspaceFile).mockClear()
})

describe('loadLocalImage resolve override', () => {
  it('calls the override with the raw src, bypassing readWorkspaceFile', async () => {
    const resolve = vi.fn().mockResolvedValue('data:image/png;base64,AAAA')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '', resolve }

    const result = await loadLocalImage(asset, 'chats/c1/attachments/x.png')

    expect(result).toBe('data:image/png;base64,AAAA')
    expect(resolve).toHaveBeenCalledWith('chats/c1/attachments/x.png')
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('still returns null for a self-loading src without calling the override', async () => {
    const resolve = vi.fn()
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '', resolve }

    expect(await loadLocalImage(asset, 'https://example.com/a.png')).toBeNull()
    expect(resolve).not.toHaveBeenCalled()
  })

  it('falls back to readWorkspaceFile when no override is set (pre-existing behaviour)', async () => {
    vi.mocked(readWorkspaceFile).mockResolvedValue('\x89PNG')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: 'docs' }

    const result = await loadLocalImage(asset, 'logo.png')

    expect(readWorkspaceFile).toHaveBeenCalledWith('ws1', 'docs/logo.png')
    expect(result).toMatch(/^data:image\/png;base64,/)
  })

  it('returns null when there is no asset (pre-existing early return)', async () => {
    expect(await loadLocalImage(null, 'logo.png')).toBeNull()
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('returns null for an empty src (pre-existing early return)', async () => {
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '' }
    expect(await loadLocalImage(asset, '')).toBeNull()
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('returns null for a non-image extension (pre-existing early return)', async () => {
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '' }
    expect(await loadLocalImage(asset, 'notes.txt')).toBeNull()
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('utf8-encodes an svg read (pre-existing behaviour)', async () => {
    vi.mocked(readWorkspaceFile).mockResolvedValue('<svg></svg>')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '' }

    const result = await loadLocalImage(asset, 'icon.svg')

    expect(result).toBe(`data:image/svg+xml;utf8,${encodeURIComponent('<svg></svg>')}`)
  })

  it('returns null when readWorkspaceFile rejects (pre-existing behaviour)', async () => {
    vi.mocked(readWorkspaceFile).mockRejectedValue(new Error('not found'))
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '' }

    expect(await loadLocalImage(asset, 'missing.png')).toBeNull()
  })
})

describe('resolveAssetPath', () => {
  it('treats a leading slash as workspace-root-relative, ignoring fileDir', () => {
    expect(resolveAssetPath('docs/nested', '/assets/logo.png')).toBe('assets/logo.png')
  })

  it('pops a segment on ".."', () => {
    expect(resolveAssetPath('docs/nested', '../logo.png')).toBe('docs/logo.png')
  })

  it('drops "." and empty segments', () => {
    expect(resolveAssetPath('docs', './logo.png')).toBe('docs/logo.png')
  })
})

describe('useMarkdownAsset', () => {
  it('returns null with no provider (pre-existing default)', () => {
    const { result } = renderHook(() => useMarkdownAsset())
    expect(result.current).toBeNull()
  })

  it('returns the provided asset info', () => {
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: 'docs' }
    const { result } = renderHook(() => useMarkdownAsset(), {
      wrapper: ({ children }) =>
        createElement(MarkdownAssetContext.Provider, { value: asset }, children),
    })
    expect(result.current).toBe(asset)
  })
})

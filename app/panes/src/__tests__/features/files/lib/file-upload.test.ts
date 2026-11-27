import { afterEach, describe, it, expect, vi } from 'vitest'

const { createFileNode, writeDocument } = vi.hoisted(() => ({
  createFileNode: vi.fn(async () => {}),
  writeDocument: vi.fn(async (_ref: unknown, _content: string, _options: unknown) => {}),
}))
vi.mock('@/features/files/lib/file-tree-api', () => ({ createFileNode }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  writeDocument,
}))

import { pickAndUploadFiles } from '@/features/files/lib/file-upload'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef } from '@/lib/api/documents'

describe('pickAndUploadFiles', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    setActiveWorkspaceId(null)
  })

  it('creates and writes each picked file in the workspace it is given', async () => {
    setActiveWorkspaceId('ws-in-view')
    vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(function (
      this: HTMLInputElement,
    ) {
      Object.defineProperty(this, 'files', { value: [new File(['hi'], 'a.txt')] })
      this.onchange?.(new Event('change'))
    })

    const written = await pickAndUploadFiles('ws-b', 'docs')

    expect(written).toEqual(['docs/a.txt'])
    expect(createFileNode).toHaveBeenCalledExactlyOnceWith('ws-b', 'docs/a.txt', 'file')
    expect(writeDocument).toHaveBeenCalledExactlyOnceWith(
      workspaceRef('ws-b', 'docs/a.txt'),
      'aGk=',
      { encoding: 'base64' },
    )
  })

  it.each([
    ['binary bytes (invalid UTF-8 + NUL)', [0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0xfe, 0x01, 0x80]],
    ['an empty file', []],
  ])('uploads %s byte-for-byte as base64', async (_name, bytes) => {
    vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(function (
      this: HTMLInputElement,
    ) {
      Object.defineProperty(this, 'files', {
        value: [new File([new Uint8Array(bytes)], 'f.bin')],
      })
      this.onchange?.(new Event('change'))
    })

    await pickAndUploadFiles('ws-b', '')

    const content = writeDocument.mock.calls.at(-1)?.[1] ?? ''
    const decoded = Uint8Array.from(atob(content), (c) => c.charCodeAt(0))
    expect(Array.from(decoded)).toEqual(bytes)
  })
})

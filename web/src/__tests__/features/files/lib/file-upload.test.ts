import { afterEach, describe, it, expect, vi } from 'vitest'

const { createFileNode, writeFileContent } = vi.hoisted(() => ({
  createFileNode: vi.fn(async () => {}),
  writeFileContent: vi.fn(async () => {}),
}))
vi.mock('@/features/files/lib/file-tree-api', () => ({ createFileNode, writeFileContent }))

import { pickAndUploadFiles, readFileAsBase64 } from '@/features/files/lib/file-upload'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// Uploads go through readFileAsBase64 → writeFileContent(..., 'base64') so the
// exact bytes reach the daemon. This pins the byte-faithful half: bytes that are
// invalid UTF-8 and contain NULs must survive the encode (file.text() would have
// mangled them, which was the corruption this fixes).
describe('readFileAsBase64', () => {
  it('encodes exact bytes, binary-safe (invalid UTF-8 + NUL)', async () => {
    const bytes = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0xfe, 0x01, 0x80])
    const b64 = await readFileAsBase64(new Blob([bytes]))

    // Decode the base64 back to bytes and compare byte-for-byte.
    const decoded = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
    expect(Array.from(decoded)).toEqual(Array.from(bytes))
  })

  it('an empty file encodes to an empty payload', async () => {
    expect(await readFileAsBase64(new Blob([]))).toBe('')
  })
})

// An upload creates files, so it goes to the workspace of the tree it was asked from,
// whichever other workspace happens to be in view.
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
    expect(writeFileContent).toHaveBeenCalledExactlyOnceWith('ws-b', 'docs/a.txt', 'aGk=', 'base64')
  })
})

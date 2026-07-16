import { describe, it, expect } from 'vitest'
import { readFileAsBase64 } from '@/features/files/lib/file-upload'

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

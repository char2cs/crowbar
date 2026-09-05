import { describe, expect, it } from 'vitest'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'

describe('extractDroppedFilePaths', () => {
  it('returns [] — a browser DataTransfer never carries a real host path; see useTauriFileDrop for where a real Tauri-desktop drop path comes from', () => {
    const dt = new DataTransfer()
    expect(extractDroppedFilePaths(dt)).toEqual([])
  })
})

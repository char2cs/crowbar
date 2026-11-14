import { describe, expect, it } from 'vitest'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'

describe('getLanguageIdFromPath', () => {
  it('detects Angular templates by file name, ahead of plain html', () => {
    expect(getLanguageIdFromPath('/tmp/app.component.html')).toBe('angular')
    expect(getLanguageIdFromPath('/tmp/app.ng.html')).toBe('angular')
    expect(getLanguageIdFromPath('/tmp/index.html')).toBe('html')
  })
})

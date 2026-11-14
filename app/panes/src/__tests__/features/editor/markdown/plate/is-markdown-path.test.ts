import { describe, expect, it } from 'vitest'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'

describe('isMarkdownPath', () => {
  it('matches .md and .markdown case-insensitively', () => {
    expect(isMarkdownPath('/a/README.md')).toBe(true)
    expect(isMarkdownPath('/a/notes.MARKDOWN')).toBe(true)
  })
  it('excludes .mdx and non-markdown', () => {
    expect(isMarkdownPath('/a/page.mdx')).toBe(false)
    expect(isMarkdownPath('/a/main.ts')).toBe(false)
    expect(isMarkdownPath('/a/markdown')).toBe(false) // no extension
  })
})

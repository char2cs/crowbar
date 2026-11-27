import { describe, expect, it } from 'vitest'
import { parseAttachmentLang } from '@/features/agent/composer/plate/attachments/attachment-lang'

describe('parseAttachmentLang', () => {
  it('parses a valid text-attachment tag', () => {
    expect(parseAttachmentLang('text-attachment:AbC123xy')).toEqual({
      kind: 'text-attachment',
      id: 'AbC123xy',
    })
  })

  it('parses a valid excalidraw tag', () => {
    expect(parseAttachmentLang('excalidraw:AbC123xy')).toEqual({
      kind: 'excalidraw',
      id: 'AbC123xy',
    })
  })

  it('rejects a bare tag with no id — ordinary discussion of the feature must not be hijacked', () => {
    expect(parseAttachmentLang('text-attachment')).toBeNull()
    expect(parseAttachmentLang('excalidraw')).toBeNull()
  })

  it('rejects an id shorter than 6 characters', () => {
    expect(parseAttachmentLang('text-attachment:abc')).toBeNull()
  })

  it('rejects an id with characters outside [A-Za-z0-9_-]', () => {
    expect(parseAttachmentLang('text-attachment:abc def!')).toBeNull()
  })

  it('rejects an unrelated language tag', () => {
    expect(parseAttachmentLang('python')).toBeNull()
  })

  it('rejects a wrong kind with a valid id', () => {
    expect(parseAttachmentLang('python:AbC123xy')).toBeNull()
    expect(parseAttachmentLang('markdown:AbC123xy')).toBeNull()
  })

  it('rejects null/undefined', () => {
    expect(parseAttachmentLang(null)).toBeNull()
    expect(parseAttachmentLang(undefined)).toBeNull()
  })
})

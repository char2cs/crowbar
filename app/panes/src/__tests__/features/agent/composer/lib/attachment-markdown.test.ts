import { describe, expect, it } from 'vitest'
import {
  textAttachmentMarkdown,
  imageMarkdown,
  excalidrawMarkdown,
  markdownForUpload,
} from '@/features/agent/composer/lib/attachment-markdown'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'

function expectRoundTripPreservesContent(
  markdown: string,
  originalContent: string,
  expectedLanguageTag: string,
) {
  const parsed = chatMarkdownToValue(markdown)

  expect(parsed.length).toBeGreaterThan(0)

  const reserialized = chatValueToMarkdown(parsed)

  expect(reserialized).toContain(originalContent)
  expect(reserialized).toContain(expectedLanguageTag)

  expect(markdown).toMatch(/^`{3,}/)
}

describe('textAttachmentMarkdown', () => {
  it('fences the raw text with an id-suffixed language tag', () => {
    const md = textAttachmentMarkdown('abc123', 'hello\nworld')
    expect(md).toContain('text-attachment:abc123')
    expect(md).toContain('hello\nworld')
  })

  it('handles text with leading/trailing whitespace', () => {
    const md = textAttachmentMarkdown('xyz789', '  content  ')
    expectRoundTripPreservesContent(md, '  content  ', 'text-attachment:xyz789')
  })

  it('handles inline backticks safely', () => {
    const md = textAttachmentMarkdown('id1', 'code: `foo()`')
    expectRoundTripPreservesContent(md, 'code: `foo()`', 'text-attachment:id1')
  })

  it('CRITICAL: handles standalone backtick fence line within content (would corrupt without escalation)', () => {
    const dangerous = 'before\n```javascript\nconst x = 1;\n```\nafter'
    const md = textAttachmentMarkdown('abc123', dangerous)

    expectRoundTripPreservesContent(md, dangerous, 'text-attachment:abc123')

    const fence = md.split('\n')[0].match(/^`+/)?.[0]
    expect(fence?.length).toBeGreaterThan(3)
  })

  it('escalates fence length: 4 backticks in content -> 5+ fence', () => {
    const content = 'line1\n````\nline2'
    const md = textAttachmentMarkdown('id1', content)

    expectRoundTripPreservesContent(md, content, 'text-attachment:id1')

    const fence = md.split('\n')[0].match(/^`+/)?.[0]
    expect(fence?.length).toBeGreaterThanOrEqual(5)
  })

  it('escalates fence length: 10 backticks in content -> 11+ fence', () => {
    const content = 'start\n' + '`'.repeat(10) + '\nend'
    const md = textAttachmentMarkdown('id2', content)

    expectRoundTripPreservesContent(md, content, 'text-attachment:id2')

    const fence = md.split('\n')[0].match(/^`+/)?.[0]
    expect(fence?.length).toBeGreaterThanOrEqual(11)
  })

  it('handles text with special markdown characters', () => {
    const md = textAttachmentMarkdown('id3', '# Heading\n[link](url)\n![img](path)')
    expectRoundTripPreservesContent(
      md,
      '# Heading\n[link](url)\n![img](path)',
      'text-attachment:id3',
    )
  })

  it('handles empty text', () => {
    const md = textAttachmentMarkdown('id4', '')
    expectRoundTripPreservesContent(md, '', 'text-attachment:id4')
  })

  it('handles text with only newlines', () => {
    const md = textAttachmentMarkdown('id5', '\n\n')
    expectRoundTripPreservesContent(md, '\n\n', 'text-attachment:id5')
  })

  it('handles ids with dashes and underscores', () => {
    const md = textAttachmentMarkdown('abc-123_def', 'text')
    expect(md).toContain('text-attachment:abc-123_def')
    expectRoundTripPreservesContent(md, 'text', 'text-attachment:abc-123_def')
  })

  it('preserves mixed backtick content correctly', () => {
    const content = 'single`backtick\n``double\n```triple```\n````quad'
    const md = textAttachmentMarkdown('id_mixed', content)
    expectRoundTripPreservesContent(md, content, 'text-attachment:id_mixed')
  })
})

describe('imageMarkdown', () => {
  it('produces a standard markdown image', () => {
    expect(imageMarkdown('diagram', 'chats/c1/attachments/x-a.png')).toBe(
      '![diagram](chats/c1/attachments/x-a.png)',
    )
  })

  it('handles empty alt text', () => {
    expect(imageMarkdown('', 'path/to/image.png')).toBe('![](path/to/image.png)')
  })

  it('handles alt text with special characters', () => {
    expect(imageMarkdown('alt text with spaces', 'image.png')).toBe(
      '![alt text with spaces](image.png)',
    )
  })

  it('handles alt text with brackets', () => {
    expect(imageMarkdown('[alt]', 'image.png')).toBe('![[alt]](image.png)')
  })

  it('handles alt text with parentheses', () => {
    expect(imageMarkdown('alt (text)', 'image.png')).toBe('![alt (text)](image.png)')
  })

  it('handles ref with query parameters', () => {
    expect(imageMarkdown('img', 'path/to/image.png?v=123')).toBe('![img](path/to/image.png?v=123)')
  })

  it('handles ref with hash fragment', () => {
    expect(imageMarkdown('img', 'path/to/image.png#section')).toBe(
      '![img](path/to/image.png#section)',
    )
  })
})

function fileLink(filename: string, ref: string) {
  return markdownForUpload({ filename, ref, contentType: 'application/pdf' })
}

describe('markdownForUpload for a non-image file', () => {
  it('produces a standard markdown link', () => {
    expect(fileLink('report.pdf', 'chats/c1/attachments/x-report.pdf')).toBe(
      '[report.pdf](chats/c1/attachments/x-report.pdf)',
    )
  })

  it('handles filename with spaces', () => {
    expect(fileLink('my report.pdf', 'path/to/file.pdf')).toBe('[my report.pdf](path/to/file.pdf)')
  })

  it('handles filename with special characters', () => {
    expect(fileLink('report-2024-09-04.docx', 'path/to/file.docx')).toBe(
      '[report-2024-09-04.docx](path/to/file.docx)',
    )
  })

  it('handles filename with brackets', () => {
    expect(fileLink('[file].txt', 'path/to/file.txt')).toBe('[[file].txt](path/to/file.txt)')
  })

  it('handles filename with parentheses', () => {
    expect(fileLink('file (1).txt', 'path/to/file.txt')).toBe('[file (1).txt](path/to/file.txt)')
  })

  it('handles ref with query parameters', () => {
    expect(fileLink('file.pdf', 'path/to/file.pdf?download=true')).toBe(
      '[file.pdf](path/to/file.pdf?download=true)',
    )
  })

  it('handles empty filename', () => {
    expect(fileLink('', 'path/to/file.txt')).toBe('[](path/to/file.txt)')
  })
})

describe('excalidrawMarkdown', () => {
  it('fences the scene JSON with an id-suffixed language tag', () => {
    const md = excalidrawMarkdown('abc123', '{"type":"excalidraw"}')
    expect(md).toContain('excalidraw:abc123')
    expect(md).toContain('{"type":"excalidraw"}')
  })

  it('handles multiline scene JSON with round-trip', () => {
    const sceneJson = '{\n  "type": "excalidraw",\n  "version": 2\n}'
    const md = excalidrawMarkdown('xyz789', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:xyz789')
  })

  it('handles scene JSON with inline backticks safely', () => {
    const sceneJson = '{"code": "`backtick`"}'
    const md = excalidrawMarkdown('id1', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:id1')
  })

  it('CRITICAL: handles scene JSON that could encode standalone fence line', () => {
    const sceneJson = '{"text":"line1\\n```\\nline2"}'
    const md = excalidrawMarkdown('id2', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:id2')
  })

  it('escalates fence length for JSON with 4+ backticks', () => {
    const sceneJson = '{"code":"' + '`'.repeat(4) + '"}'
    const md = excalidrawMarkdown('id3', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:id3')

    const fence = md.split('\n')[0].match(/^`+/)?.[0]
    expect(fence?.length).toBeGreaterThanOrEqual(5)
  })

  it('handles ids with dashes and underscores', () => {
    const sceneJson = '{"type":"excalidraw"}'
    const md = excalidrawMarkdown('abc-123_def', sceneJson)
    expect(md).toContain('excalidraw:abc-123_def')
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:abc-123_def')
  })

  it('handles complex scene JSON', () => {
    const sceneJson =
      '{"elements":[{"id":"a","type":"rectangle"},{"id":"b","type":"text","text":"hello"}]}'
    const md = excalidrawMarkdown('scene1', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:scene1')
  })

  it('handles empty JSON', () => {
    const md = excalidrawMarkdown('id4', '{}')
    expectRoundTripPreservesContent(md, '{}', 'excalidraw:id4')
  })

  it('preserves multiline JSON structure with backticks', () => {
    const sceneJson = `{
  "elements": [
    {"id": "a", "text": "\`code\`"}
  ]
}`
    const md = excalidrawMarkdown('complex_id', sceneJson)
    expectRoundTripPreservesContent(md, sceneJson, 'excalidraw:complex_id')
  })
})

describe('markdownForUpload', () => {
  it('produces image markdown for an image contentType', () => {
    expect(
      markdownForUpload({
        filename: 'shot.png',
        ref: 'chats/c1/attachments/x-shot.png',
        contentType: 'image/png',
      }),
    ).toBe('![shot.png](chats/c1/attachments/x-shot.png)')
  })

  it('produces file-link markdown for a non-image contentType', () => {
    expect(
      markdownForUpload({
        filename: 'notes.txt',
        ref: 'chats/c1/attachments/x-notes.txt',
        contentType: 'text/plain',
      }),
    ).toBe('[notes.txt](chats/c1/attachments/x-notes.txt)')
  })
})

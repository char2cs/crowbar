import { describe, expect, it } from 'vitest'
import {
  textAttachmentMarkdown,
  imageMarkdown,
  fileMarkdown,
  excalidrawMarkdown,
} from '@/features/agent/composer/lib/attachment-markdown'

describe('textAttachmentMarkdown', () => {
  it('fences the raw text with an id-suffixed language tag', () => {
    expect(textAttachmentMarkdown('abc123', 'hello\nworld')).toBe(
      '```text-attachment:abc123\nhello\nworld\n```',
    )
  })

  it('handles text with leading/trailing whitespace', () => {
    expect(textAttachmentMarkdown('xyz789', '  content  ')).toBe(
      '```text-attachment:xyz789\n  content  \n```',
    )
  })

  it('handles text with backticks', () => {
    expect(textAttachmentMarkdown('id1', 'code: `foo()`')).toBe(
      '```text-attachment:id1\ncode: `foo()`\n```',
    )
  })

  it('handles text with multiple backticks', () => {
    expect(textAttachmentMarkdown('id2', '```javascript\ncode\n```')).toBe(
      '```text-attachment:id2\n```javascript\ncode\n```\n```',
    )
  })

  it('handles text with special markdown characters', () => {
    expect(textAttachmentMarkdown('id3', '# Heading\n[link](url)\n![img](path)')).toBe(
      '```text-attachment:id3\n# Heading\n[link](url)\n![img](path)\n```',
    )
  })

  it('handles empty text', () => {
    expect(textAttachmentMarkdown('id4', '')).toBe('```text-attachment:id4\n\n```')
  })

  it('handles text with only newlines', () => {
    expect(textAttachmentMarkdown('id5', '\n\n')).toBe('```text-attachment:id5\n\n\n\n```')
  })

  it('handles ids with dashes and underscores', () => {
    expect(textAttachmentMarkdown('abc-123_def', 'text')).toBe(
      '```text-attachment:abc-123_def\ntext\n```',
    )
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
    expect(imageMarkdown('img', 'path/to/image.png?v=123')).toBe(
      '![img](path/to/image.png?v=123)',
    )
  })

  it('handles ref with hash fragment', () => {
    expect(imageMarkdown('img', 'path/to/image.png#section')).toBe(
      '![img](path/to/image.png#section)',
    )
  })
})

describe('fileMarkdown', () => {
  it('produces a standard markdown link', () => {
    expect(fileMarkdown('report.pdf', 'chats/c1/attachments/x-report.pdf')).toBe(
      '[report.pdf](chats/c1/attachments/x-report.pdf)',
    )
  })

  it('handles filename with spaces', () => {
    expect(fileMarkdown('my report.pdf', 'path/to/file.pdf')).toBe(
      '[my report.pdf](path/to/file.pdf)',
    )
  })

  it('handles filename with special characters', () => {
    expect(fileMarkdown('report-2024-09-04.docx', 'path/to/file.docx')).toBe(
      '[report-2024-09-04.docx](path/to/file.docx)',
    )
  })

  it('handles filename with brackets', () => {
    expect(fileMarkdown('[file].txt', 'path/to/file.txt')).toBe(
      '[[file].txt](path/to/file.txt)',
    )
  })

  it('handles filename with parentheses', () => {
    expect(fileMarkdown('file (1).txt', 'path/to/file.txt')).toBe(
      '[file (1).txt](path/to/file.txt)',
    )
  })

  it('handles ref with query parameters', () => {
    expect(fileMarkdown('file.pdf', 'path/to/file.pdf?download=true')).toBe(
      '[file.pdf](path/to/file.pdf?download=true)',
    )
  })

  it('handles empty filename', () => {
    expect(fileMarkdown('', 'path/to/file.txt')).toBe('[](path/to/file.txt)')
  })
})

describe('excalidrawMarkdown', () => {
  it('fences the scene JSON with an id-suffixed language tag', () => {
    expect(excalidrawMarkdown('abc123', '{"type":"excalidraw"}')).toBe(
      '```excalidraw:abc123\n{"type":"excalidraw"}\n```',
    )
  })

  it('handles multiline scene JSON', () => {
    const sceneJson = '{\n  "type": "excalidraw",\n  "version": 2\n}'
    expect(excalidrawMarkdown('xyz789', sceneJson)).toBe(
      `\`\`\`excalidraw:xyz789\n${sceneJson}\n\`\`\``,
    )
  })

  it('handles scene JSON with backticks', () => {
    expect(excalidrawMarkdown('id1', '{"code": "`backtick`"}')).toBe(
      '```excalidraw:id1\n{"code": "`backtick`"}\n```',
    )
  })

  it('handles scene JSON with fence markers', () => {
    expect(excalidrawMarkdown('id2', '{"text": "```code```"}')).toBe(
      '```excalidraw:id2\n{"text": "```code```"}\n```',
    )
  })

  it('handles ids with dashes and underscores', () => {
    expect(excalidrawMarkdown('abc-123_def', '{"type":"excalidraw"}')).toBe(
      '```excalidraw:abc-123_def\n{"type":"excalidraw"}\n```',
    )
  })

  it('handles complex scene JSON', () => {
    const sceneJson =
      '{"elements":[{"id":"a","type":"rectangle"},{"id":"b","type":"text","text":"hello"}]}'
    expect(excalidrawMarkdown('scene1', sceneJson)).toBe(
      `\`\`\`excalidraw:scene1\n${sceneJson}\n\`\`\``,
    )
  })

  it('handles empty JSON', () => {
    expect(excalidrawMarkdown('id3', '{}')).toBe('```excalidraw:id3\n{}\n```')
  })
})
